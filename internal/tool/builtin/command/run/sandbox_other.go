//go:build !darwin && !windows

package run

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func (p *fixedCommand) configureSandbox(a arguments, permissions tool.ExecutionPermissions, _ tool.ProtectedData) error {
	if permissions.SandboxMode != "" && !permissions.SandboxMode.Valid() {
		return tool.ErrDenied
	}
	p.boundary = tool.CommandBoundary{Mode: permissions.SandboxMode, Escalated: a.SandboxPermissions == sandboxEscalated, Risk: commandRisk(p.command)}
	if permissions.SandboxMode == tool.SandboxFullAccess {
		p.boundary.Escalated = false
		return nil
	}
	projectWrite := a.SandboxPermissions == sandboxProjectWrite
	if projectWrite && (permissions.SandboxMode != tool.SandboxWorkspaceWrite || p.workspace == nil) {
		return tool.ErrDenied
	}
	if p.boundary.Escalated || permissions.SandboxMode == "" {
		return nil
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
