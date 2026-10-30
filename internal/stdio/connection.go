package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// connection 持有输入输出，关闭必须能够中断正在进行的读写。
// 有界队列统一串行输出，防止多个执行交叉写入一条 JSON。
type connection struct {
	ctx          context.Context
	cancel       context.CancelCauseFunc
	in           io.ReadCloser
	out          io.WriteCloser
	queue        chan []byte
	done         chan struct{}
	writeTimeout time.Duration
	closeOnce    sync.Once
	closeErr     error
}

func newConnection(ctx context.Context, in io.ReadCloser, out io.WriteCloser, timeout time.Duration) *connection {
	ctx, cancel := context.WithCancelCause(ctx)
	return &connection{ctx: ctx, cancel: cancel, in: in, out: out, queue: make(chan []byte, outputQueueSize), done: make(chan struct{}), writeTimeout: timeout}
}

func (c *connection) closeStreams() {
	c.closeOnce.Do(func() { c.closeErr = errors.Join(c.in.Close(), c.out.Close()) })
}

func (c *connection) send(value any) error {
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

func (c *connection) writeLoop() {
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
