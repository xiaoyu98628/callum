package executor

import "github.com/xiaoyu98628/callum/internal/tool"

// Targets 返回固定实际目标的副本；命令工作目录不属于文件目标。
func (p *Prepared) Targets() []tool.Target {
	var targets []tool.Target
	if p.target != nil {
		targets = append(targets, *p.target)
	}
	if p.plan != nil {
		targets = append(targets, p.plan.Targets...)
	}
	return targets
}
