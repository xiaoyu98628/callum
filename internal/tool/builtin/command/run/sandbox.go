package run

import "github.com/xiaoyu98628/callum/internal/tool"

// sandboxRequest 统一各平台的固定边界判定；handled 表示请求无需进入系统沙箱装配。
// 此处不创建资源，各平台继续独立实现隔离、权限和清理。
func (p *fixedCommand) sandboxRequest(a arguments, permissions tool.ExecutionPermissions) (projectWrite, handled bool, err error) {
	if permissions.SandboxMode != "" && !permissions.SandboxMode.Valid() {
		return false, true, tool.ErrDenied
	}
	p.boundary = tool.CommandBoundary{Mode: permissions.SandboxMode, Escalated: a.SandboxPermissions == sandboxEscalated, Risk: commandRisk(p.command)}
	if permissions.SandboxMode == tool.SandboxFullAccess {
		p.boundary.Escalated = false
		return false, true, nil
	}
	projectWrite = a.SandboxPermissions == sandboxProjectWrite
	if projectWrite && (permissions.SandboxMode != tool.SandboxWorkspaceWrite || p.workspace == nil) {
		return false, true, tool.ErrDenied
	}
	return projectWrite, p.boundary.Escalated || permissions.SandboxMode == "", nil
}
