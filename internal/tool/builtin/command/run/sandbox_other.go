//go:build !darwin && !windows

package run

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func (p *fixedCommand) configureSandbox(a arguments, permissions tool.ExecutionPermissions, _ tool.ProtectedData) error {
	projectWrite, handled, err := p.sandboxRequest(a, permissions)
	if err != nil || handled {
		return err
	}
	if projectWrite {
		return tool.ErrUnavailable
	}
	p.boundary.Risk = "sandbox_backend_unsupported"
	return nil
}

func (*fixedCommand) setupExecution(context.Context) (func() error, error) {
	return func() error { return nil }, nil
}

func (*fixedCommand) sandboxBackend() string { return "" }
