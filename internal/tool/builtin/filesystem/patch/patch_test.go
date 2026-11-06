package patch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

func patchTestExecutor(t *testing.T) *executor.Executor {
	t.Helper()
	r := registry.New()
	if err := r.Register(New()); err != nil {
		t.Fatal(err)
	}
	e, err := executor.New(r, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func patchTestArgs(t *testing.T, body string, versions ...patchVersion) json.RawMessage {
	t.Helper()
	if versions == nil {
		versions = []patchVersion{}
	}
	raw, err := json.Marshal(patchArguments{Patch: "*** Begin Patch\n" + body + "\n*** End Patch\n", BaseVersions: versions})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func patchTestPrepare(t *testing.T, e *executor.Executor, scope tool.Scope, raw json.RawMessage) *executor.Prepared {
	t.Helper()
	p, err := e.Prepare(t.Context(), scope, "apply_patch", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return p
}
func patchTestWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func patchTestRead(t *testing.T, path string) string {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func TestPatchCreatesAndUpdatesWithBoundVersions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.txt")
	before := "header\r\nold\r\ntail"
	patchTestWrite(t, path, before)
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	e := patchTestExecutor(t)
	scope := tool.Scope{ID: "workspace", RootPath: dir}
	args := patchTestArgs(t, "*** Update File: old.txt\n@@\n header\n-old\n+new\n tail\n*** Add File: nested/note.txt\n+创建的内容", patchVersion{"old.txt", textSHA256([]byte(before))})
	p := patchTestPrepare(t, e, scope, args)
	if p.OutsideWorkspace() || policy.Evaluate(policy.OnRequest, p.Definition().Effects, false).Action != policy.Allow {
		t.Fatal("普通修改未使用工作区策略")
	}
	if _, err := os.Stat(filepath.Join(dir, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("准备阶段创建了目录")
	}
	result, err := e.Execute(t.Context(), p, p.Hash())
	if err != nil || result.Outcome != tool.OutcomeApplied {
		t.Fatalf("补丁执行失败: %s %+v %v", result.Output, result, err)
	}
	if patchTestRead(t, path) != "header\r\nnew\r\ntail" || patchTestRead(t, filepath.Join(dir, "nested", "note.txt")) != "创建的内容\n" {
		t.Fatal("文件内容错误")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 && info.Mode().Perm() != 0666 {
		t.Fatalf("改变了已有权限: %v", info.Mode())
	} // Windows 权限由平台测试验证 DACL。
	var report patchReport
	if json.Unmarshal(result.Output, &report) != nil || len(report.Changes) != 2 || len(report.CreatedDirectories) != 1 || len(report.RecoveryFiles) != 0 {
		t.Fatalf("结果审计错误: %s", result.Output)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".callum-patch-") {
			t.Fatal("成功后遗留恢复副本")
		}
	}
}

func TestPatchPreflightRejectsWholeBatchOnConflict(t *testing.T) {
	for _, kind := range []string{"version", "context", "existing_create", "ambiguous"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			one, two := filepath.Join(dir, "one"), filepath.Join(dir, "two")
			patchTestWrite(t, one, "header\nold\ntail\n")
			patchTestWrite(t, two, "a\na\n")
			hash := textSHA256([]byte("a\na\n"))
			body := "*** Update File: two\n@@\n-a\n+b"
			if kind == "version" {
				hash = strings.Repeat("0", 64)
				body = "*** Update File: two\n@@\n a\n-a\n+b"
			}
			if kind == "context" {
				body = "*** Update File: two\n@@\n-missing\n+b"
			}
			versions := []patchVersion{{"one", textSHA256([]byte(patchTestRead(t, one)))}, {"two", hash}}
			if kind == "existing_create" {
				body = "*** Add File: two\n+x"
				versions = versions[:1]
			}
			e := patchTestExecutor(t)
			args := patchTestArgs(t, "*** Update File: one\n@@\n header\n-old\n+new\n tail\n"+body, versions...)
			p, err := e.Prepare(t.Context(), tool.Scope{ID: "w", RootPath: dir}, "apply_patch", args)
			if err == nil {
				defer p.Close()
				var result tool.Result
				result, err = e.Execute(t.Context(), p, p.Hash())
				if kind == "version" {
					var report patchReport
					actualHash := textSHA256([]byte(patchTestRead(t, two)))
					if json.Unmarshal(result.Output, &report) != nil || report.Changes[1].BeforeSHA256 != actualHash || report.Changes[1].ObservedSHA256 != actualHash {
						t.Fatalf("把请求前提伪造成实际文件版本: %s", result.Output)
					}
				}
				if result.Outcome != tool.OutcomeNotApplied {
					t.Fatalf("冲突前已产生副作用: %+v", result)
				}
			}
			if !errors.Is(err, tool.ErrConflict) {
				t.Fatalf("没有拒绝冲突: %v", err)
			}
			if patchTestRead(t, one) != "header\nold\ntail\n" || patchTestRead(t, two) != "a\na\n" {
				t.Fatal("冲突导致部分覆盖")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 2 {
				t.Fatal("冲突遗留辅助文件")
			}
		})
	}
}

func TestPatchPlanAndPolicyBindEveryTarget(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	external := filepath.Join(outside, "new.txt")
	e := patchTestExecutor(t)
	raw := patchTestArgs(t, "*** Add File: inside.txt\n+x\n*** Add File: "+external+"\n+y")
	p := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, raw)
	var request executor.Operation
	if json.Unmarshal(p.Request(), &request) != nil || request.Plan == nil || len(request.Plan.Targets) != 2 {
		t.Fatalf("缺少多目标快照: %s", p.Request())
	}
	if !p.OutsideWorkspace() || request.Plan.Targets[0].WithinWorkspace == request.Plan.Targets[1].WithinWorkspace {
		t.Fatal("未按全部实际目标判断范围")
	}
	if policy.Evaluate(policy.OnRequest, p.Definition().Effects, p.OutsideWorkspace()).Action != policy.Ask || policy.Evaluate(policy.Never, p.Definition().Effects, p.OutsideWorkspace()).Action != policy.Deny {
		t.Fatal("工作区外目标绕过审批")
	}
	// 相同固定操作不能因为辅助名字或参数表顺序变化而改变指纹。
	again := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, raw)
	if again.Hash() != p.Hash() || !reflect.DeepEqual(again.Request(), p.Request()) {
		t.Fatal("相同操作的重试指纹不稳定")
	}
	request.Plan.Targets[0].Path = external
	var fresh executor.Operation
	_ = json.Unmarshal(p.Request(), &fresh)
	if fresh.Plan.Targets[0].Path == external {
		t.Fatal("调用方修改了固定计划")
	}
	if _, err := e.Execute(t.Context(), p, "changed"); !errors.Is(err, tool.ErrDenied) {
		t.Fatal("未核对审批指纹")
	}
}

func TestPatchRefusesReplacedTargetsAndVersionChanges(t *testing.T) {
	for _, kind := range []string{"file", "parent", "content", "new_target"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "note")
			before := "head\nold\ntail\n"
			patchTestWrite(t, path, before)
			e := patchTestExecutor(t)
			body := "*** Update File: note\n@@\n head\n-old\n+new\n tail"
			versions := []patchVersion{{"note", textSHA256([]byte(before))}}
			if kind == "new_target" {
				body = "*** Add File: future\n+x"
				versions = nil
			}
			p := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, patchTestArgs(t, body, versions...))
			switch kind {
			case "file":
				if err := os.Rename(path, path+"-old"); err != nil {
					t.Fatal(err)
				}
				patchTestWrite(t, path, "user changes")
			case "parent":
				if err := os.Rename(dir, dir+"-old"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(dir + "-old") })
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				patchTestWrite(t, path, "user changes")
			case "content":
				patchTestWrite(t, path, "user changes")
			case "new_target":
				patchTestWrite(t, filepath.Join(dir, "future"), "user changes")
			}
			result, err := e.Execute(t.Context(), p, p.Hash())
			if err == nil || result.Outcome != tool.OutcomeNotApplied {
				t.Fatalf("替换后仍执行: %s %v", result.Output, err)
			}
			if kind == "new_target" {
				path = filepath.Join(dir, "future")
			}
			if patchTestRead(t, path) != "user changes" {
				t.Fatal("覆盖了用户改动")
			}
		})
	}
}

// 在看到第一个目标已经生效后才取消，验证真实写入边界，不依赖时间或轮询。
type cancelAfterPatch struct {
	context.Context
	path, value string
	cancel      context.CancelFunc
}

func (c *cancelAfterPatch) Err() error {
	data, _ := os.ReadFile(c.path)
	if string(data) == c.value {
		c.cancel()
	}
	return c.Context.Err()
}
func TestPatchCancellationPreservesAppliedFilesAndRecoveryCopies(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "one"), filepath.Join(dir, "two")
	before := "head\nold\ntail\n"
	for _, path := range []string{first, second} {
		patchTestWrite(t, path, before)
	}
	instance := New()
	raw := patchTestArgs(t, "*** Update File: one\n@@\n head\n-old\n+new\n tail\n*** Update File: two\n@@\n head\n-old\n+new\n tail", patchVersion{"one", textSHA256([]byte(before))}, patchVersion{"two", textSHA256([]byte(before))})
	normalized, err := instance.Validate(raw)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := instance.Prepare(t.Context(), tool.Scope{ID: "w", RootPath: dir}, normalized)
	if err != nil {
		t.Fatal(err)
	}
	defer inv.Close()
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &cancelAfterPatch{Context: base, path: first, value: "head\nnew\ntail\n", cancel: cancel}
	result, err := inv.Execute(ctx)
	if !errors.Is(err, context.Canceled) || result.Outcome != tool.OutcomePartial {
		t.Fatalf("取消覆盖操作事实: %s %s %v", result.Outcome, result.Output, err)
	}
	if patchTestRead(t, first) != "head\nnew\ntail\n" || patchTestRead(t, second) != before {
		t.Fatal("取消后继续修改其他文件")
	}
	var report patchReport
	if json.Unmarshal(result.Output, &report) != nil || report.Changes[0].Outcome != tool.OutcomeApplied || report.Changes[1].Outcome != tool.OutcomeNotApplied {
		t.Fatalf("缺少逐文件事实: %s", result.Output)
	}
	if len(report.RecoveryFiles) != 2 {
		t.Fatalf("取消后缺少恢复内容: %s", result.Output)
	}
	contents := map[string]bool{}
	for _, path := range report.RecoveryFiles {
		contents[patchTestRead(t, path)] = true
	}
	if !contents[before] || !contents["head\nnew\ntail\n"] {
		t.Fatal("恢复副本不包含原文及预期内容")
	}
}

func TestPatchLargeReplacementRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	patchTestWrite(t, filepath.Join(dir, "note"), "old\n")
	e := patchTestExecutor(t)
	p := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, patchTestArgs(t, "*** Update File: note\n@@\n-old\n+new", patchVersion{"note", textSHA256([]byte("old\n"))}))
	if policy.Evaluate(policy.OnRequest, p.Definition().Effects, false).Action != policy.Ask || policy.Evaluate(policy.Never, p.Definition().Effects, false).Action != policy.Deny {
		t.Fatal("整文件覆盖被归为普通修改")
	}
}

func TestPatchValidationRejectsMissingOrExtraVersions(t *testing.T) {
	instance := New()
	for _, raw := range []json.RawMessage{
		patchTestArgs(t, "*** Update File: a\n@@\n-x\n+y"),
		patchTestArgs(t, "*** Add File: a\n+x", patchVersion{"a", strings.Repeat("0", 64)}),
		json.RawMessage(`{"patch":"x","patch":"y","base_versions":[]}`),
		json.RawMessage(`{"patch":"x","base_versions":[],"effects":["read"]}`),
	} {
		if _, err := instance.Validate(raw); err == nil {
			t.Fatalf("接受无效版本或权限参数: %s", raw)
		}
	}
	if _, err := patchTestExecutor(t).Prepare(t.Context(), tool.Scope{}, "apply_patch", patchTestArgs(t, "*** Add File: local.txt\n+x")); !errors.Is(err, tool.ErrPathBase) {
		t.Fatalf("未绑定工作区时使用了进程目录: %v", err)
	}
}

func TestPatchArgumentsRejectAmbiguousJSON(t *testing.T) {
	instance := New()
	valid := string(patchTestArgs(t, "*** Update File: a\n@@\n-x\n+y", patchVersion{"a", strings.Repeat("0", 64)}))
	for _, raw := range []string{
		valid + ` {}`,
		strings.Replace(valid, `"patch":`, `"Patch":`, 1),
		strings.Replace(valid, `"path":"a"`, `"Path":"a"`, 1),
		strings.Replace(valid, `"path":"a"`, `"path":"a","path":"b"`, 1),
		strings.Replace(valid, `"path":"a"`, `"path":"a","unknown":true`, 1),
		strings.Replace(valid, `"path":"a"`, `"path":null`, 1),
		strings.Replace(valid, `"path":"a"`, `"path":1`, 1),
		strings.Replace(valid, `"base_versions":[`, `"base_versions":[null,`, 1),
	} {
		if _, err := instance.Validate(json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("接受无效补丁参数: %s, %v", raw, err)
		}
		invocation, err := instance.Prepare(t.Context(), tool.Scope{}, json.RawMessage(raw))
		if invocation != nil {
			if closeErr := invocation.Close(); closeErr != nil {
				t.Error(closeErr)
			}
		}
		if !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("准备阶段接受无效补丁参数: %s, %v", raw, err)
		}
	}
}

func TestConcurrentPatchesCannotOverwriteEachOther(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note")
	before := "head\nold\ntail\n"
	patchTestWrite(t, path, before)
	e := patchTestExecutor(t)
	raw := patchTestArgs(t, "*** Update File: note\n@@\n head\n-old\n+new\n tail", patchVersion{"note", textSHA256([]byte(before))})
	first := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, raw)
	second := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, raw)
	type answer struct {
		result tool.Result
		err    error
	}
	done := make(chan answer, 2)
	for _, prepared := range []*executor.Prepared{first, second} {
		go func() { result, err := e.Execute(t.Context(), prepared, prepared.Hash()); done <- answer{result, err} }()
	}
	success, conflicts := 0, 0
	for range 2 {
		answer := <-done
		if answer.err == nil && answer.result.Outcome == tool.OutcomeApplied {
			success++
		} else if errors.Is(answer.err, tool.ErrConflict) && answer.result.Outcome == tool.OutcomeNotApplied {
			conflicts++
		} else {
			t.Fatalf("并发结果错误: %+v %v", answer.result, answer.err)
		}
	}
	if success != 1 || conflicts != 1 || patchTestRead(t, path) != "head\nnew\ntail\n" {
		t.Fatalf("并发覆盖或重复执行: success=%d conflict=%d", success, conflicts)
	}
}

func TestPatchSymlinksUseActualTargetsAndRejectRedirects(t *testing.T) {
	for _, kind := range []string{"update_link", "new_parent_link"} {
		t.Run(kind, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			original, other := filepath.Join(outside, "original"), filepath.Join(outside, "other")
			before := "head\nold\ntail\n"
			patchTestWrite(t, original, before)
			patchTestWrite(t, other, "unrelated")
			link := filepath.Join(dir, "link")
			e := patchTestExecutor(t)
			var p *executor.Prepared
			if kind == "update_link" {
				if err := os.Symlink(original, link); err != nil {
					t.Skipf("无法创建符号链接: %v", err)
				}
				p = patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, patchTestArgs(t, "*** Update File: link\n@@\n head\n-old\n+new\n tail", patchVersion{"link", textSHA256([]byte(before))}))
				if !p.OutsideWorkspace() {
					t.Fatal("链接实际目标在工作区外却自动放行")
				}
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, link); err != nil {
					t.Fatal(err)
				}
			} else {
				p = patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, patchTestArgs(t, "*** Add File: link/new\n+data"))
				if err := os.Symlink(outside, link); err != nil {
					t.Skipf("无法创建符号链接: %v", err)
				}
			}
			result, err := e.Execute(t.Context(), p, p.Hash())
			if err == nil || result.Outcome != tool.OutcomeNotApplied {
				t.Fatalf("链接重定向后仍执行: %s %v", result.Output, err)
			}
			if patchTestRead(t, original) != before || patchTestRead(t, other) != "unrelated" {
				t.Fatal("修改了替换后的目标")
			}
			if _, err := os.Stat(filepath.Join(outside, "new")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("创建文件逃出固定祖先")
			}
		})
	}
}

func TestPatchCanonicalArgumentsHaveStableApprovalHash(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{"a", "b"} {
		patchTestWrite(t, filepath.Join(dir, path), "head\nold\ntail\n")
	}
	body := "*** Update File: a\n@@\n head\n-old\n+new\n tail\n*** Update File: b\n@@\n head\n-old\n+new\n tail"
	hash := textSHA256([]byte("head\nold\ntail\n"))
	e := patchTestExecutor(t)
	scope := tool.Scope{ID: "w", RootPath: dir}
	one := patchTestPrepare(t, e, scope, patchTestArgs(t, body, patchVersion{"a", hash}, patchVersion{"b", hash}))
	two := patchTestPrepare(t, e, scope, patchTestArgs(t, strings.ReplaceAll(body, "\n", "\r\n"), patchVersion{"b", strings.ToUpper(hash)}, patchVersion{"a", hash}))
	if one.Hash() != two.Hash() {
		t.Fatal("等价格式或版本顺序改变了操作指纹")
	}
}

type replaceCreatedPatchParent struct {
	context.Context
	dir     string
	changed bool
	err     error
}

func (c *replaceCreatedPatchParent) Err() error {
	if !c.changed {
		path := filepath.Join(c.dir, "created")
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			c.changed = true
			if err := os.Rename(path, path+"-original"); err != nil {
				c.err = err
				return err
			}
			c.err = os.Mkdir(path, 0700)
		}
	}
	if c.err != nil {
		return c.err
	}
	return c.Context.Err()
}
func TestPatchDoesNotContinueThroughReplacedCreatedParent(t *testing.T) {
	dir := t.TempDir()
	instance := New()
	raw, err := instance.Validate(patchTestArgs(t, "*** Add File: created/nested/note\n+data"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := instance.Prepare(t.Context(), tool.Scope{ID: "w", RootPath: dir}, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer inv.Close()
	ctx := &replaceCreatedPatchParent{Context: t.Context(), dir: dir}
	result, err := inv.Execute(ctx)
	if ctx.err != nil {
		t.Fatal(ctx.err)
	}
	if !errors.Is(err, tool.ErrDenied) || result.Outcome != tool.OutcomePartial || !ctx.changed {
		t.Fatalf("父目录被替换后继续了操作: %s %v", result.Output, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "created", "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("修改了替换后的父目录")
	}
}
