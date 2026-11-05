package settings_test

import (
	"encoding/json"
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
		`{}`, `null`, `{"enabled":null,"message_threshold":128,"byte_threshold":4194304,"retain_turns":4}`,
		`{"enabled":true,"enabled":false,"message_threshold":128,"byte_threshold":4194304,"retain_turns":4}`,
		`{"enabled":true,"message_threshold":257,"byte_threshold":4194304,"retain_turns":4}`,
		`{"enabled":true,"message_threshold":128,"byte_threshold":1023,"retain_turns":4}`,
		`{"enabled":true,"message_threshold":128,"byte_threshold":4194304,"retain_turns":0}`,
		`{"enabled":true,"message_threshold":128,"byte_threshold":4194304,"retain_turns":4,"model_id":"override"}`,
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

func TestLegacyCompactionSnapshotRemainsHistorical(t *testing.T) {
	raw := []byte(`{"revision":7,"enabled":true,"message_threshold":64,"byte_threshold":2000,"retain_turns":2}`)
	var old settings.CompactionSnapshot
	if err := json.Unmarshal(raw, &old); err != nil || !old.Valid() {
		t.Fatalf("旧快照不可读取: %v", err)
	}
	encoded, err := json.Marshal(old)
	if err != nil || string(encoded) != string(raw) {
		t.Fatalf("历史策略被改写: %s %v", encoded, err)
	}
	if _, err := settings.DecodeCompaction(raw); err == nil {
		t.Fatal("旧阈值进入当前设置接口")
	}
	upgraded, changed, err := settings.UpgradeCompaction(nil)
	if err == nil || changed {
		t.Fatal("非法迁移输入使用默认设置")
	}
	upgraded, changed, err = settings.UpgradeCompaction([]byte(`{"enabled":false,"message_threshold":64,"byte_threshold":2000,"retain_turns":2}`))
	if err != nil || !changed || upgraded.Enabled || upgraded.RetainTurns != 2 || upgraded.TokenBudget != 0 {
		t.Fatal("旧策略升级丢失开关或虚构 Token 预算")
	}
}
