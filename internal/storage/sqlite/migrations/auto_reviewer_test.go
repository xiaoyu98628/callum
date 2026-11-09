package migrations_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func TestAutoReviewerMigrationPreservesExecutionAndAudit(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	conv := schema.Conversation{ID: id(), Title: "已有会话"}
	snapshot := settings.Snapshot{Revision: 1, Execution: settings.DefaultExecution()}
	run := schema.Run{ID: id(), ConversationID: conv.ID, Status: "completed", FinishedAt: new(int64(1)), SettingsSnapshot: &snapshot}
	step := schema.Step{ID: id(), ConversationID: conv.ID, RunID: run.ID, Sequence: 1, Type: "tool_call", Name: "旧工具", Status: "completed", FinishedAt: new(int64(1))}
	approval := schema.Approval{ID: id(), RunID: run.ID, StepID: step.ID, Request: []byte(`{}`), RequestHash: strings.Repeat("a", 64), Status: "approved", DecidedBy: new("local_user"), DecidedAt: new(int64(1))}
	for _, row := range []any{&conv, &run, &step, &approval} {
		if err := db.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DB.Exec("CREATE TRIGGER preserved_run_trigger AFTER UPDATE OF status ON conversation_runs BEGIN SELECT 1; END").Error; err != nil {
		t.Fatal(err)
	}
	var ddl string
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE name='conversation_runs'").Scan(&ddl).Error; err != nil {
		t.Fatal(err)
	}
	var objects []string
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE tbl_name='conversation_runs' AND type IN ('index','trigger') AND sql IS NOT NULL").Scan(&objects).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(ddl, "COALESCE(json_extract(settings_snapshot,'$.approvals_reviewer') IN ('user','auto_review'),0)", "json_extract(settings_snapshot,'$.approvals_reviewer') IS 'user'", 1)
	if ddl == legacy {
		t.Fatal("缺少当前审批约束")
	}
	pool, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	legacy = strings.Replace(legacy, "conversation_runs", "conversation_runs_v2", 1)
	queries := []string{legacy, "INSERT INTO conversation_runs_v2 SELECT * FROM conversation_runs", "DROP TABLE conversation_runs", "ALTER TABLE conversation_runs_v2 RENAME TO conversation_runs"}
	queries = append(queries, objects...)
	queries = append(queries, "PRAGMA user_version=2")
	for _, query := range queries {
		if _, err := pool.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}

	// 插入已有版本的孤儿记录，验证重建表及版本在升级失败后一起回滚。
	orphan := id()
	if _, err := pool.ExecContext(ctx, "UPDATE conversation_runs SET conversation_id=? WHERE id=?", orphan, run.ID); err != nil {
		t.Fatal(err)
	}
	if rejected, err := sqlite.Open(ctx, path); err == nil {
		rejected.Close()
		t.Fatal("允许外键损坏的升级")
	}
	var version int
	if err := pool.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatalf("版本未回滚: %d %v", version, err)
	}
	if _, err := pool.ExecContext(ctx, "UPDATE conversation_runs SET conversation_id=? WHERE id=?", conv.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	upgraded, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var old schema.Run
	if err := upgraded.DB.Where("id=?", run.ID).Take(&old).Error; err != nil || old.SettingsSnapshot == nil || old.SettingsSnapshot.ApprovalsReviewer != policy.User {
		t.Fatalf("旧执行未保留: %+v %v", old, err)
	}
	var count int
	if err := upgraded.DB.Raw("SELECT count(*) FROM conversation_run_approvals WHERE id=? AND decided_by='local_user'", approval.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("旧审批未保留: %d %v", count, err)
	}
	if err := upgraded.DB.Raw("SELECT count(*) FROM sqlite_schema WHERE name='preserved_run_trigger'").Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("触发器未保留: %d %v", count, err)
	}
	snapshot.ApprovalsReviewer = policy.AutoReview
	next := schema.Run{ID: id(), ConversationID: conv.ID, Status: "running", SettingsSnapshot: &snapshot}
	if err := upgraded.DB.Create(&next).Error; err != nil {
		t.Fatalf("升级后不能保存 AI 审批快照: %v", err)
	}
}
