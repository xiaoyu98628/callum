package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	maxRequestBytes     = 1 << 20
	maxResponseBytes    = 8 << 20
	outputQueueSize     = 64
	defaultWriteTimeout = 5 * time.Second
)

var errSlowConsumer = errors.New("协议输出阻塞超过时限")

// Connection 持有标准流、取消信号和有界输出队列，统一串行输出协议消息。
// 创建者必须停止请求处理及事件生产者后再调用 Close。
type Connection struct {
	parent       context.Context
	ctx          context.Context
	cancel       context.CancelCauseFunc
	in           io.ReadCloser
	out          io.WriteCloser
	queue        chan []byte
	done         chan struct{}
	writeTimeout time.Duration
	closeOnce    sync.Once
	closeErr     error
	watchDone    chan struct{}
	stopWatch    func() bool
	finishOnce   sync.Once
	finishErr    error
}

// NewConnection 接管输入输出并启动写入循环，Close 必须能够中断底层 Read/Write。
// 外部取消或通信失败会关闭管道，创建失败时输入输出仍由调用方持有。
func NewConnection(ctx context.Context, in io.ReadCloser, out io.WriteCloser) (*Connection, error) {
	if in == nil || out == nil {
		return nil, errors.New("协议输入输出不能为空")
	}
	parent := ctx
	ctx, cancel := context.WithCancelCause(ctx)
	c := &Connection{parent: parent, ctx: ctx, cancel: cancel, in: in, out: out, queue: make(chan []byte, outputQueueSize), done: make(chan struct{}), writeTimeout: defaultWriteTimeout, watchDone: make(chan struct{})}
	c.stopWatch = context.AfterFunc(ctx, func() { c.closeStreams(); close(c.watchDone) })
	go c.writeLoop()
	return c, nil
}

// Context 在父上下文取消或通信失败时结束，可作为执行管理器的运行上下文。
func (c *Connection) Context() context.Context { return c.ctx }

// Close 在连接正常时排空输出，关闭管道并保留通信与清理错误，可重复调用。
// 调用前必须停止 Serve 与所有事件生产者，防止关闭队列后继续发送事件。
func (c *Connection) Close() error {
	c.finishOnce.Do(func() {
		close(c.queue)
		<-c.done
		if !c.stopWatch() {
			<-c.watchDone
		}
		c.closeStreams()
		cause := context.Cause(c.ctx)
		if c.parent.Err() != nil && errors.Is(cause, context.Cause(c.parent)) {
			cause = nil
		}
		c.finishErr = errors.Join(cause, c.closeErr)
		c.cancel(context.Canceled)
	})
	return c.finishErr
}

// 通信取消造成的读写错误由 Close 统一报告，避免覆盖原因或重复返回同一故障。
func (c *Connection) normalizeError(err error) error {
	if c.ctx.Err() != nil && (errors.Is(err, context.Cause(c.ctx)) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrClosed)) {
		return nil
	}
	return err
}
func (c *Connection) closeStreams() {
	c.closeOnce.Do(func() { c.closeErr = errors.Join(c.in.Close(), c.out.Close()) })
}

func (c *Connection) send(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		c.cancel(errors.New("协议响应编码失败"))
		return err
	}
	if len(data) > maxResponseBytes {
		err := errors.New("协议响应超过大小限制")
		c.cancel(err)
		return err
	}
	data = append(data, '\n')
	timer := time.NewTimer(c.writeTimeout)
	defer timer.Stop()
	select {
	case <-c.ctx.Done():
		return context.Cause(c.ctx)
	case c.queue <- data:
		return nil
	case <-timer.C:
		c.cancel(errSlowConsumer)
		return errSlowConsumer
	}
}

func (c *Connection) writeLoop() {
	defer close(c.done)
	for data := range c.queue {
		if c.ctx.Err() != nil {
			return
		}
		// 即使队列尚未填满，也限制单条消息的写入等待；超时关闭管道以解除阻塞。
		timer := time.AfterFunc(c.writeTimeout, func() { c.cancel(errSlowConsumer) })
		_, err := io.Copy(c.out, bytes.NewReader(data))
		timer.Stop()
		if err != nil {
			c.cancel(fmt.Errorf("写入协议输出失败: %w", err))
			return
		}
	}
}
