package app

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestGeneralSettingsThroughStdioAndRestart(t *testing.T) {
	runtime, cfg := runtimeFixture(t, 2)
	first, second := sharedPeer(t, runtime), sharedPeer(t, runtime)
	read := func(p *appPeer) settings.Record {
		t.Helper()
		res := p.call(t, "settings.get", map[string]any{"key": settings.GeneralKey})
		requireOK(t, res)
		var value settings.Record
		if err := json.Unmarshal(res.Result, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	initial := read(first)
	if value, err := settings.DecodeGeneralRecord(initial); err != nil || value != settings.DefaultGeneral() || initial.Revision != 1 {
		t.Fatalf("常规默认值异常: %s, %v", initial.Value, err)
	}
	stale := read(second)
	for _, raw := range []string{
		`{}`, `{"follow_up_mode":"unknown"}`, `{"follow_up_mode":null}`,
		`{"follow_up_mode":"queue","follow_up_mode":"steer"}`,
		`{"follow_up_mode":"queue","extra":true}`,
	} {
		res := first.call(t, "settings.update", map[string]any{"key": settings.GeneralKey, "revision": initial.Revision, "value": json.RawMessage(raw)})
		if res.Error == nil || res.Error.Code != -32602 {
			t.Fatalf("接受了非法常规设置 %s: %+v", raw, res)
		}
	}
	update := func(p *appPeer, rev int64, mode string) wireFrame {
		t.Helper()
		return p.call(t, "settings.update", map[string]any{"key": settings.GeneralKey, "revision": rev, "value": map[string]string{"follow_up_mode": mode}})
	}
	requireOK(t, update(first, initial.Revision, "steer"))
	if res := update(second, stale.Revision, "queue"); res.Error == nil || res.Error.Code != 1003 {
		t.Fatalf("旧版本写入未报告冲突: %+v", res)
	}
	first.stop(t)
	second.stop(t)
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(t.Context(), filepath.Join(cfg.DataDir, "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	var stored struct {
		Value    string
		Revision int64
	}
	if err := db.DB.WithContext(t.Context()).Table("app_settings").Where("key=?", settings.GeneralKey).Take(&stored).Error; err != nil ||
		stored.Value != `{"follow_up_mode":"steer"}` || stored.Revision != initial.Revision+1 {
		t.Fatalf("重启后常规设置未保留: %+v, %v", stored, err)
	}
}
