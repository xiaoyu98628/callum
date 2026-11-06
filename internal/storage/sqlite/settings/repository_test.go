package settings_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/settings/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func fixture(t *testing.T) (*sqlite.Database, *service.Service) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repository, err := store.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	s, err := service.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	return db, s
}

func TestLegacySettingsCompatibilityAndSandboxRevision(t *testing.T) {
	db, s := fixture(t)
	legacy := `{"approval_policy":"on-request","approvals_reviewer":"user"}`
	if err := db.DB.Table("app_settings").Where("key=?", settings.ExecutionKey).Update("value", legacy).Error; err != nil {
		t.Fatal(err)
	}
	current, err := s.Get(t.Context(), settings.ExecutionKey)
	if err != nil || current.Revision != 1 {
		t.Fatalf("旧设置读取失败: %+v %v", current, err)
	}
	snapshot, err := settings.ExecutionSnapshot(current)
	if err != nil || snapshot.SandboxMode != tool.SandboxWorkspaceWrite {
		t.Fatalf("旧配置没有返回明确默认值: %+v %v", snapshot, err)
	}
	var stored string
	if err := db.DB.Table("app_settings").Where("key=?", settings.ExecutionKey).Select("value").Scan(&stored).Error; err != nil || stored != legacy {
		t.Fatalf("查询改写了旧存储: %s %v", stored, err)
	}
	saved, err := s.Update(t.Context(), settings.ExecutionKey, 1, json.RawMessage(`{"approval_policy":"on-request","approvals_reviewer":"user","sandbox_mode":"read-only"}`))
	if err != nil || saved.Revision != 2 {
		t.Fatalf("沙箱设置保存失败: %+v %v", saved, err)
	}
	saved, err = s.Update(t.Context(), settings.ExecutionKey, 2, json.RawMessage(legacy))
	if err != nil || saved.Revision != 3 {
		t.Fatalf("旧客户端更新失败: %+v %v", saved, err)
	}
	snapshot, err = settings.ExecutionSnapshot(saved)
	if err != nil || snapshot.SandboxMode != tool.SandboxReadOnly {
		t.Fatalf("旧客户端更新扩大了权限: %+v %v", snapshot, err)
	}
	if _, err := s.Update(t.Context(), settings.ExecutionKey, 2, json.RawMessage(legacy)); !errors.Is(err, settings.ErrConflict) {
		t.Fatalf("旧版本覆盖了沙箱设置: %v", err)
	}
}

func TestConcurrentSettingsUpdateAndRollback(t *testing.T) {
	db, s := fixture(t)
	current, err := s.Get(t.Context(), settings.ExecutionKey)
	if err != nil {
		t.Fatal(err)
	}
	id, err := uuid.Parse(current.ID)
	if err != nil || id.Version() != 7 || current.Revision != 1 {
		t.Fatalf("默认设置不正确: %+v", current)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, choice := range []string{"always", "never"} {
		wg.Go(func() {
			_, err := s.Update(t.Context(), settings.ExecutionKey, current.Revision, json.RawMessage(`{"approval_policy":"`+choice+`","approvals_reviewer":"user"}`))
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, settings.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("并发更新没有互斥: success=%d conflicts=%d", success, conflicts)
	}
	before, err := s.Get(t.Context(), settings.ExecutionKey)
	if err != nil || before.Revision != 2 {
		t.Fatalf("设置版本不正确: %+v %v", before, err)
	}
	// 只在临时数据库注入写入失败，验证版本和值一起回滚，没有内存配置提前生效。
	if err := db.DB.Exec(`CREATE TRIGGER reject_settings BEFORE UPDATE ON app_settings BEGIN SELECT RAISE(ABORT,'simulated failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(t.Context(), settings.ExecutionKey, before.Revision, json.RawMessage(`{"approval_policy":"on-request","approvals_reviewer":"user"}`)); !errors.Is(err, settings.ErrStorage) {
		t.Fatalf("没有返回持久化失败: %v", err)
	}
	after, err := s.Get(t.Context(), settings.ExecutionKey)
	if err != nil || after.Revision != before.Revision || !bytes.Equal(after.Value, before.Value) {
		t.Fatalf("失败写入改动了设置: %+v %v", after, err)
	}
	if err := db.DB.Exec("DROP TRIGGER reject_settings").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(t.Context(), settings.ExecutionKey, before.Revision, json.RawMessage(`{"approval_policy":"on-request","approvals_reviewer":"user"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceUsesTransactionalSettingsSnapshot(t *testing.T) {
	db, s := fixture(t)
	conversations, err := conversationstore.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	newID := func() string { return uuid.Must(uuid.NewV7()).String() }
	input := func() conversation.Begin {
		return conversation.Begin{Create: true, ConversationID: newID(), RunID: newID(), StepID: newID(), UserMessageID: newID(), AssistantMessageID: newID(),
			Model: model.Snapshot{ID: newID(), RemoteModelID: "test", Provider: model.ProviderSnapshot{ID: newID(), Name: "test"}}, Protocol: "chatcompletions", Content: []model.Block{{Type: "text", Text: "问题"}}}
	}
	first, err := conversations.Begin(t.Context(), input())
	if err != nil || first.Run.SettingsSnapshot == nil || first.Run.SettingsSnapshot.Revision != 1 {
		t.Fatalf("受理未保存快照: %+v %v", first.Run, err)
	}
	if _, err := s.Update(t.Context(), settings.ExecutionKey, 1, json.RawMessage(`{"approval_policy":"always","approvals_reviewer":"user"}`)); err != nil {
		t.Fatal(err)
	}
	second, err := conversations.Begin(t.Context(), input())
	if err != nil || second.Run.SettingsSnapshot == nil || second.Run.SettingsSnapshot.Revision != 2 || second.Run.SettingsSnapshot.ApprovalPolicy != "always" {
		t.Fatalf("新执行未使用已提交设置: %+v %v", second.Run, err)
	}
	if first.Run.SettingsSnapshot.ApprovalPolicy != "on-request" {
		t.Fatal("旧执行快照跟随全局配置发生变化")
	}
	// 模拟数据库内容异常；受理失败不能留下会话、消息、运行或步骤。
	if err := db.DB.Table("app_settings").Where("key=?", settings.ExecutionKey).Update("value", `{"approval_policy":"invalid","approvals_reviewer":"user"}`).Error; err != nil {
		t.Fatal(err)
	}
	bad := input()
	if _, err := conversations.Begin(t.Context(), bad); !errors.Is(err, settings.ErrStorage) {
		t.Fatalf("坏配置没有阻止受理: %v", err)
	}
	for table, column := range map[string]string{"conversations": "id", "conversation_messages": "conversation_id", "conversation_runs": "conversation_id", "conversation_run_steps": "conversation_id"} {
		var count int64
		if err := db.DB.Table(table).Where(column+"=?", bad.ConversationID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s 留下不完整受理: %d %v", table, count, err)
		}
	}
}
