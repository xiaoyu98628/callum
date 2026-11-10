package run

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type invocation struct {
	fixed           *fixedCommand
	mu              sync.Mutex
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
	closeOnce       sync.Once
	closeErr        error
}

func (*invocation) Target() *tool.Target                    { return nil }
func (p *invocation) Snapshot() json.RawMessage             { return slices.Clone(p.fixed.snapshot) }
func (p *invocation) CommandBoundary() tool.CommandBoundary { return p.fixed.boundary }
func (p *invocation) Execute(ctx context.Context) (tool.Result, error) {
	return p.ExecuteStream(ctx, nil)
}

func (p *invocation) ExecuteStream(ctx context.Context, emit func(tool.OutputChunk) error) (tool.Result, error) {
	p.mu.Lock()
	if p.started || p.closed {
		p.mu.Unlock()
		return tool.Result{Outcome: tool.OutcomeNotApplied}, tool.ErrDenied
	}
	p.started = true
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel, p.done = cancel, make(chan struct{})
	p.mu.Unlock()
	defer func() { cancel(); close(p.done) }()
	if err := p.fixed.check(runCtx); err != nil {
		return tool.Result{Outcome: tool.OutcomeNotApplied}, err
	}
	cleanup, err := p.fixed.setupExecution(runCtx)
	if err != nil {
		outcome := tool.OutcomeNotApplied
		if errors.Is(err, tool.ErrUnknownOutcome) {
			outcome = tool.OutcomeUnknown
		}
		return tool.Result{Outcome: outcome}, err
	}
	report, err := runProcess(runCtx, p.fixed, emit)
	if cleanupErr := cleanup(); cleanupErr != nil {
		report.CleanupComplete = false
		err = errors.Join(err, cleanupErr)
	}
	output, marshalErr := json.Marshal(report)
	outcome := tool.OutcomeNotApplied
	if report.Started {
		outcome = tool.OutcomeUnknown
		if err == nil && report.CleanupComplete && report.ExitCode != nil && report.Signal == "" {
			// applied 只表示本次命令已执行至正常退出；业务成功由退出码和输出表达。
			outcome = tool.OutcomeApplied
		}
	}
	return tool.Result{Output: output, Outcome: outcome}, errors.Join(err, marshalErr)
}

func (p *invocation) Close() error {
	p.mu.Lock()
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	done := p.done
	p.mu.Unlock()
	if done != nil {
		<-done
	}
	p.closeOnce.Do(func() { p.closeErr = p.fixed.close() })
	return p.closeErr
}
