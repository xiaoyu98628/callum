package run

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const cleanupTimeout = 2 * time.Second

type processReport struct {
	Started         bool       `json:"started"`
	PID             *int       `json:"pid"`
	ExitCode        *int       `json:"exit_code"`
	Signal          string     `json:"signal,omitempty"`
	TimedOut        bool       `json:"timed_out"`
	Cancelled       bool       `json:"cancelled"`
	CleanupComplete bool       `json:"cleanup_complete"`
	Stdout          outputData `json:"stdout"`
	Stderr          outputData `json:"stderr"`
	StreamTruncated bool       `json:"stream_truncated"`
}

type processExit struct {
	code   int
	signal string
}

func runProcess(ctx context.Context, fixed *fixedCommand, emit func(tool.OutputChunk) error) (report processReport, retErr error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &capture{emit: emit, cancel: cancel}
	defer func() { report.Stdout, report.Stderr, report.StreamTruncated = output.results() }()
	var files []*os.File
	defer func() {
		var closeErr error
		for _, file := range files {
			closeErr = errors.Join(closeErr, file.Close())
		}
		retErr = errors.Join(retErr, closeErr)
		if closeErr != nil {
			report.CleanupComplete = false
		}
	}()
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return report, errors.Join(tool.ErrUnavailable, err)
	}
	files = append(files, stdin)
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return report, errors.Join(tool.ErrUnavailable, err)
	}
	files = append(files, stdout)
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		return report, errors.Join(tool.ErrUnavailable, err, stdoutWriter.Close())
	}
	files = append(files, stderr)
	if err := fixed.check(ctx); err != nil {
		return report, errors.Join(err, stdoutWriter.Close(), stderrWriter.Close())
	}
	process, startErr := startProcess(fixed, stdin, stdoutWriter, stderrWriter)
	// 仅子进程持有写端；父进程必须及时关闭，否则读取者永远等不到 EOF。
	closeWriters := errors.Join(stdoutWriter.Close(), stderrWriter.Close())
	if process == nil {
		return report, errors.Join(tool.ErrUnavailable, startErr, closeWriters)
	}
	report.Started = true
	report.PID = new(process.processID())
	defer func() {
		closeErr := process.close()
		retErr = errors.Join(retErr, closeErr)
		if closeErr != nil {
			report.CleanupComplete = false
		}
	}()
	readers := make(chan error, 2)
	go func() { readers <- drain(stdout, "stdout", output) }()
	go func() { readers <- drain(stderr, "stderr", output) }()
	// 两个读取任务只等待本次管道；终止进程后仍不结束时关闭读端并等待它们返回。
	exit, waitErr := waitProcess(runCtx, process)
	stopErr := process.stop()
	retErr = errors.Join(startErr, closeWriters, waitErr, stopErr)
	cleanupErr := errors.Join(startErr, closeWriters, stopErr)
	if waitErr != nil {
		// 调用方取消、超时或输出回调失败均结束本次进程组/Job，再有界等待主进程。
		cleanup, end := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		exit, err = waitProcess(cleanup, process)
		end()
		retErr = errors.Join(retErr, err)
		cleanupErr = errors.Join(cleanupErr, err)
		if err == nil {
			report.ExitCode, report.Signal = new(exit.code), exit.signal
		}
	} else {
		report.ExitCode, report.Signal = new(exit.code), exit.signal
	}
	if outputErr := output.failure(); outputErr != nil {
		// 内部取消只是停止进程的手段，不将发布失败伪装成用户取消。
		retErr = errors.Join(outputErr, startErr, closeWriters, stopErr, err)
	}
	report.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	report.Cancelled = errors.Is(ctx.Err(), context.Canceled)
	if exit.signal != "" && retErr == nil {
		retErr = tool.ErrUnknownOutcome
	}
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()
	for remaining := 2; remaining > 0; {
		select {
		case readErr := <-readers:
			retErr = errors.Join(retErr, readErr)
			cleanupErr = errors.Join(cleanupErr, readErr)
			remaining--
		case <-timer.C:
			// 后代保留管道或主动脱离进程组时，不让读取任务拖住运行时关闭。
			retErr = errors.Join(retErr, tool.ErrUnavailable)
			cleanupErr = errors.Join(cleanupErr, tool.ErrUnavailable)
			retErr = errors.Join(retErr, stdout.Close(), stderr.Close())
			files = files[:1] // 读端已经关闭，由此处收回所有权，避免重复关闭掩盖错误。
			for ; remaining > 0; remaining-- {
				retErr = errors.Join(retErr, <-readers)
			}
		}
	}
	report.CleanupComplete = report.ExitCode != nil && cleanupErr == nil
	return report, retErr
}

func waitProcess(ctx context.Context, process *process) (processExit, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		exit, done, err := process.poll()
		if err != nil || done {
			return exit, err
		}
		select {
		case <-ctx.Done():
			return processExit{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func drain(file *os.File, stream string, output *capture) error {
	buffer := make([]byte, tool.MaxOutputChunkBytes)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			output.write(stream, buffer[:n])
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.Join(tool.ErrUnavailable, err)
		}
	}
}
