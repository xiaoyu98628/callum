//go:build !windows

package run

import "context"

type sandboxState struct{}

func (*sandboxState) check(context.Context) error { return nil }
func (*sandboxState) close() error                { return nil }
func (*sandboxState) readableRoots() []string     { return nil }

// RecoverSandbox 清理平台沙箱遗留状态；当前平台的沙箱由系统管理。
func RecoverSandbox(context.Context, string) error { return nil }
