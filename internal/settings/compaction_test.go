package settings_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/settings"
)

func TestCompactionSettingsAreExplicitAndSnapshotIsIndependent(t *testing.T) {
	value := settings.DefaultCompaction()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := settings.DecodeCompaction(raw)
	if err != nil || parsed != value {
		t.Fatalf("默认配置无效: %+v %v", parsed, err)
	}
	for _, invalid := range []string{
		`{}`, `null`,
		strings.Replace(string(raw), `"enabled":true`, `"enabled":null`, 1),
		strings.Replace(string(raw), `"enabled":true`, `"enabled":true,"enabled":false`, 1),
		strings.Replace(string(raw), `"retain_turns":4`, `"retain_turns":0`, 1),
		strings.Replace(string(raw), `"retain_turns":4`, `"retain_turns":4,"model_id":"override"`, 1),
		string(raw) + " {}",
		`{"enabled":true,"token_budget":-1,"output_reserve":4096,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}`,
		`{"enabled":true,"token_budget":0,"output_reserve":0,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}`,
		`{"enabled":true,"token_budget":null,"output_reserve":4096,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}`,
	} {
		if _, err := settings.DecodeCompaction(json.RawMessage(invalid)); err == nil {
			t.Fatalf("接受非法压缩配置: %s", invalid)
		}
	}
	original := settings.Snapshot{Revision: 1, Execution: settings.DefaultExecution(), Compaction: &settings.CompactionSnapshot{Revision: 1, Compaction: value}}
	copy := original.Clone()
	copy.Compaction.Enabled = false
	if !original.Compaction.Enabled {
		t.Fatal("回调能修改原执行策略")
	}
}

func TestTokenBudgetUsesConfiguredCapacityAndExplicitFallback(t *testing.T) {
	cfg := settings.DefaultCompaction()
	if _, err := cfg.InputBudget(0, 0); err == nil {
		t.Fatal("未知容量被猜测为默认窗口")
	}
	if budget, err := cfg.InputBudget(10000, 0); err != nil || budget != 4880 {
		t.Fatalf("预留计算错误: %d %v", budget, err)
	}
	if budget, err := cfg.InputBudget(10000, 1000); err != nil || budget != 7976 {
		t.Fatalf("输出上限没有限制预留: %d %v", budget, err)
	}
	cfg.TokenBudget = 3000
	if budget, err := cfg.InputBudget(0, 0); err != nil || budget != 3000 {
		t.Fatal("未知窗口时显式预算失效")
	}
	cfg.TokenBudget = 99999
	if budget, err := cfg.InputBudget(10000, 0); err != nil || budget != 4880 {
		t.Fatal("显式预算放大了已知窗口")
	}
	if _, err := cfg.InputBudget(5000, 0); err == nil {
		t.Fatal("预留超过窗口仍获得正预算")
	}
}

func TestCompactionSnapshotRoundTripAndValidation(t *testing.T) {
	original := settings.CompactionSnapshot{Revision: 7, Compaction: settings.DefaultCompaction()}
	original.Enabled, original.TokenBudget, original.RetainTurns = false, 3000, 2
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var parsed settings.CompactionSnapshot
	if err := json.Unmarshal(raw, &parsed); err != nil || !parsed.Valid() || parsed != original {
		t.Fatalf("执行快照未完整保存: %+v %v", parsed, err)
	}
	for _, invalid := range []string{
		`{}`, `null`, `{"revision":7}`,
		`{"revision":7,"enabled":true,"message_threshold":64,"byte_threshold":2000,"retain_turns":2}`,
		strings.Replace(string(raw), `"revision":7,`, "", 1),
		strings.Replace(string(raw), `"revision":7`, `"revision":0`, 1),
		strings.Replace(string(raw), `"revision":7`, `"revision":null`, 1),
		strings.Replace(string(raw), `"revision":7`, `"revision":7,"revision":8`, 1),
		strings.Replace(string(raw), `"token_budget":3000`, `"token_budget":null`, 1),
		strings.Replace(string(raw), `"output_reserve":4096,`, "", 1),
		strings.Replace(string(raw), `"retain_turns":2`, `"retain_turns":0`, 1),
		strings.Replace(string(raw), `"retain_turns":2`, `"retain_turns":2,"unknown":true`, 1),
		string(raw) + " {}",
	} {
		if err := json.Unmarshal([]byte(invalid), &parsed); err == nil {
			t.Fatalf("接受无效执行快照: %s", invalid)
		}
		if parsed != original {
			t.Fatalf("解码失败改写了已有快照: %+v", parsed)
		}
	}
}
