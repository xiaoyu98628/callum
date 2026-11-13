//go:build darwin

package run

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// configureSandbox 只固定边界、路径及系统启动器，不创建文件或运行探测命令。
func (p *fixedCommand) configureSandbox(a arguments, permissions tool.ExecutionPermissions, protection tool.ProtectedData) error {
	if permissions.SandboxMode != "" && !permissions.SandboxMode.Valid() {
		return tool.ErrDenied
	}
	p.boundary = tool.CommandBoundary{Mode: permissions.SandboxMode, Escalated: a.SandboxPermissions == sandboxEscalated, Risk: commandRisk(p.command)}
	if permissions.SandboxMode == tool.SandboxFullAccess {
		// 完全访问由受理快照明确选定，模型的单次沙箱参数不改变该模式。
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
	// 未接入系统沙箱的平台保留明确的逐次审批，不能宣称受系统边界保护。
	if runtime.GOOS != "darwin" {
		if projectWrite {
			return tool.ErrUnavailable
		}
		p.boundary.Risk = "sandbox_backend_unsupported"
		return nil
	}
	var err error
	p.sandboxExecutable, err = pinPath("/usr/bin/sandbox-exec", false)
	if err != nil {
		return errors.Join(tool.ErrUnavailable, err)
	}
	p.temporaryParent, err = pinPath(os.TempDir(), true)
	if err != nil {
		return err
	}
	if protection.Directory != "" {
		p.protectedDirectory, err = pinPath(protection.Directory, true)
		if err != nil {
			return err
		}
		if protection.Contains(p.temporaryParent.actual) || (projectWrite && protection.Contains(p.workspace.actual)) {
			return tool.ErrDenied
		}
	}
	p.temporaryPath = filepath.Join(p.temporaryParent.actual, "callum-command-"+rand.Text())
	p.boundary.Sandboxed, p.boundary.Mode = true, tool.SandboxReadOnly
	if projectWrite {
		p.boundary.Mode = tool.SandboxWorkspaceWrite
		p.boundary.CreatesWorkspaceDirectory = p.createsWorkspaceDirectory()
	}
	p.profile = seatbeltProfile(projectWrite, p.protectedDirectory != nil)
	if p.protectedDirectory != nil {
		// deny 优先于 allow，必须从原有数据读取拒绝规则中排除固定的只读技能子树。
		var exclusions strings.Builder
		for i, root := range p.readOnlyDirectories {
			if (tool.ProtectedData{Directory: p.protectedDirectory.actual}).Contains(root.actual) {
				exclusions.WriteString(fmt.Sprintf(" (require-not (subpath (param \"RUNTIME_ROOT_%d\")))", i))
			}
		}
		if exclusions.Len() > 0 {
			// 原生程序解析资源绝对路径需要查询父目录元数据；根目录枚举仍单独拒绝。
			p.profile = strings.Replace(p.profile, `(deny file-read* (subpath (param "APP_DATA")))`, `(deny file-read* (require-all (subpath (param "APP_DATA")) (require-not (literal (param "APP_DATA")))`+exclusions.String()+`))`+"\n"+`(deny file-read-data (literal (param "APP_DATA")))`, 1)
		}
	}
	ancestorArguments := p.protectDataAncestors(projectWrite)
	p.launchArguments = []string{p.sandboxExecutable.actual, "-p", p.profile, "-D", "COMMAND_TEMP=" + p.temporaryPath}
	p.launchArguments = append(p.launchArguments, ancestorArguments...)
	if p.workDirectory != nil {
		p.profile += `(allow file-write* (require-all (subpath (param "RUN_WORK")) (require-not (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))))` + "\n"
		p.launchArguments = append(p.launchArguments, "-D", "RUN_WORK="+p.workDirectory.actual)
	}
	for i, root := range p.readOnlyDirectories {
		key := fmt.Sprintf("RUNTIME_ROOT_%d", i)
		p.profile += fmt.Sprintf("(deny file-write* (subpath (param \"%s\")))\n", key)
		p.launchArguments = append(p.launchArguments, "-D", key+"="+root.actual)
	}
	// 策略在所有只读资源规则拼接完成后传给启动器。
	p.launchArguments[2] = p.profile
	if p.protectedDirectory != nil {
		p.launchArguments = append(p.launchArguments, "-D", "APP_DATA="+p.protectedDirectory.actual)
	}
	if projectWrite {
		p.launchArguments = append(p.launchArguments, "-D", "WORKSPACE="+p.workspace.actual)
	}
	p.launchArguments = append(p.launchArguments, "--")
	p.launchArguments = append(p.launchArguments, p.command...)
	// 不把真实用户缓存目录自动加入写入范围。常用工具缓存转入此次受控临时目录。
	for _, key := range []string{"TMPDIR", "TMP", "TEMP", "GOCACHE", "GOTMPDIR", "npm_config_cache", "PIP_CACHE_DIR"} {
		p.environment = slices.DeleteFunc(p.environment, func(entry string) bool { return strings.HasPrefix(entry, key+"=") })
		value := p.temporaryPath
		if key == "GOCACHE" {
			value = filepath.Join(value, "go-build")
		}
		if key == "npm_config_cache" {
			value = filepath.Join(value, "npm-cache")
		}
		p.environment = append(p.environment, key+"="+value)
	}
	return nil
}

// 仅识别系统 mkdir 在项目内创建单层目录的操作，现有目标不会被覆盖。
func (p *fixedCommand) createsWorkspaceDirectory() bool {
	if len(p.command) != 2 || p.command[0] != "/bin/mkdir" {
		return false
	}
	name := p.command[1]
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." || strings.HasPrefix(name, "-") {
		return false
	}
	relative, err := filepath.Rel(p.workspace.actual, p.directory.actual)
	return err == nil && filepath.IsLocal(relative)
}

// 保护项目内通往应用数据的祖先路径，避免获批命令通过重命名祖先绕过路径限制。
func (p *fixedCommand) protectDataAncestors(projectWrite bool) []string {
	if !projectWrite {
		return nil
	}
	var arguments []string
	roots := slices.Clone(p.readOnlyDirectories)
	if p.protectedDirectory != nil {
		roots = append(roots, p.protectedDirectory)
	}
	for _, root := range roots {
		for parent := filepath.Dir(root.actual); ; parent = filepath.Dir(parent) {
			relative, err := filepath.Rel(p.workspace.actual, parent)
			if err != nil || relative == "." || !filepath.IsLocal(relative) {
				break
			}
			key := fmt.Sprintf("APP_DATA_PARENT_%d", len(arguments)/2)
			p.profile += fmt.Sprintf("(deny file-write-unlink (literal (param \"%s\")))\n", key)
			arguments = append(arguments, "-D", key+"="+parent)
		}
	}
	return arguments
}

// setupExecution 只在审批完成、路径复核后创建私有临时目录；调用者在进程树收尾后清理。
func (p *fixedCommand) setupExecution(context.Context) (func() error, error) {
	if !p.boundary.Sandboxed {
		return func() error { return nil }, nil
	}
	if err := os.Mkdir(p.temporaryPath, 0700); err != nil {
		return nil, errors.Join(tool.ErrUnavailable, err)
	}
	return func() error { return os.RemoveAll(p.temporaryPath) }, nil
}

// 路径使用 sandbox-exec 的独立参数传递，不能将带引号的目录拼进策略代码。
// 默认不开放任何网络或其他应用的 IPC；子进程继承同一策略。
func seatbeltProfile(workspaceWrite, protectData bool) string {
	profile := `(version 1)
(deny default)
(allow process-exec)
(allow process-fork)
(allow signal (target same-sandbox))
(allow process-info* (target same-sandbox))
(allow file-read*)
(allow sysctl-read)
(allow mach-lookup (global-name "com.apple.system.opendirectoryd.libinfo"))
(allow file-write-data (require-all (literal "/dev/null") (vnode-type CHARACTER-DEVICE)))
(allow file-write* (require-all
  (subpath (param "COMMAND_TEMP"))
  (require-not (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))))
`
	if workspaceWrite {
		profile += `(allow file-write* (require-all
  (subpath (param "WORKSPACE"))
  (require-not (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))))
(deny file-write-unlink (literal (param "WORKSPACE")))
`
	}
	profile += `(deny file-write-unlink (literal (param "COMMAND_TEMP")))
(deny file-write* (regex #"(^|/)[.](git|codex|agents|aws)(/|$)"))
`
	if protectData {
		profile += `(deny file-read* (subpath (param "APP_DATA")))
(deny file-write* (subpath (param "APP_DATA")))
`
	}
	return profile
}

func (*fixedCommand) sandboxBackend() string { return "macos-seatbelt" }
