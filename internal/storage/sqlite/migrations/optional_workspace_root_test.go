package migrations_test

import (
	"database/sql"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/workspace"
	"github.com/xiaoyu98628/callum/internal/workspace"
)

func versionOneProject(t *testing.T) (string, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repo, err := store.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, err := repo.Create(t.Context(), workspace.Workspace{ID: uuid.Must(uuid.NewV7()).String(), Name: "保留项目", RootPath: t.TempDir(), OtherPaths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	conv := schema.Conversation{ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: &project.ID, Title: "保留会话"}
	run := schema.Run{ID: uuid.Must(uuid.NewV7()).String(), ConversationID: conv.ID, Status: "completed", FinishedAt: new(int64(1))}
	message := schema.Message{ID: uuid.Must(uuid.NewV7()).String(), ConversationID: conv.ID, RunID: run.ID, Sequence: 1, Role: "user", Content: []model.Block{{Type: "text", Text: "迁移必须保留的消息"}}, Status: "completed", CompletedAt: new(int64(1))}
	for _, value := range []any{&conv, &run, &message} {
		if err := db.DB.WithContext(t.Context()).Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DB.Exec("CREATE TRIGGER preserved_project_trigger AFTER UPDATE OF name ON workspaces BEGIN SELECT 1; END").Error; err != nil {
		t.Fatal(err)
	}
	var ddl string
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE name='workspaces'").Scan(&ddl).Error; err != nil {
		t.Fatal(err)
	}
	var statements []string
	if err := db.DB.Raw("SELECT sql FROM sqlite_schema WHERE tbl_name='workspaces' AND type IN ('index','trigger') AND sql IS NOT NULL").Scan(&statements).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	pool, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	// 仅将当前真实表映射的主目录改回 v1 的 NOT NULL，其他结构和已有数据保持一致。
	column := regexp.MustCompile("(?i)([`\"]root_path[`\"]\\s+text)\\b")
	legacy := column.ReplaceAllString(ddl, "${1} NOT NULL")
	if legacy == ddl {
		t.Fatal("未生成 v1 主目录约束")
	}
	legacy = strings.Replace(legacy, "workspaces", "workspaces_v1", 1)
	tx, err := pool.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			t.Error(err)
		}
	}()
	queries := []string{legacy, "INSERT INTO workspaces_v1 SELECT * FROM workspaces", "DROP TABLE workspaces", "ALTER TABLE workspaces_v1 RENAME TO workspaces"}
	queries = append(queries, statements...)
	queries = append(queries, "PRAGMA user_version=1")
	for _, query := range queries {
		if _, err := tx.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path, pool, project.ID
}

func TestOptionalWorkspaceRootMigrationPreservesData(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		name := "upgrade"
		if invalid {
			name = "rollback_and_retry"
		}
		t.Run(name, func(t *testing.T) {
			path, pool, projectID := versionOneProject(t)
			orphan := uuid.Must(uuid.NewV7()).String()
			if invalid {
				if _, err := pool.ExecContext(t.Context(), "INSERT INTO workspace_additional_folders(workspace_id,path) VALUES (?,?)", orphan, t.TempDir()); err != nil {
					t.Fatal(err)
				}
				opened, err := sqlite.Open(t.Context(), path)
				if opened != nil {
					if closeErr := opened.Close(); closeErr != nil {
						t.Error(closeErr)
					}
				}
				if err == nil || !strings.Contains(err.Error(), "外键校验失败") {
					t.Fatalf("未在迁移后校验外键: %v", err)
				}
				var version, required, temporary int
				for query, target := range map[string]*int{"PRAGMA user_version": &version, `SELECT "notnull" FROM pragma_table_info('workspaces') WHERE name='root_path'`: &required, "SELECT count(*) FROM sqlite_schema WHERE name='workspaces_v2'": &temporary} {
					if err := pool.QueryRowContext(t.Context(), query).Scan(target); err != nil {
						t.Fatal(err)
					}
				}
				if version != 1 || required != 1 || temporary != 0 {
					t.Fatalf("失败后未回滚原结构: version=%d required=%d temporary=%d", version, required, temporary)
				}
				if _, err := pool.ExecContext(t.Context(), "DELETE FROM workspace_additional_folders WHERE workspace_id=?", orphan); err != nil {
					t.Fatal(err)
				}
			}
			opened, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := opened.Close(); err != nil {
					t.Error(err)
				}
			})
			var version, foreignKeys, messages, folders, triggers int
			for query, target := range map[string]*int{"PRAGMA user_version": &version, "PRAGMA foreign_keys": &foreignKeys, "SELECT count(*) FROM conversation_messages WHERE content LIKE '%迁移必须保留的消息%'": &messages, "SELECT count(*) FROM workspace_additional_folders": &folders, "SELECT count(*) FROM sqlite_schema WHERE name='preserved_project_trigger'": &triggers} {
				if err := opened.DB.Raw(query).Scan(target).Error; err != nil {
					t.Fatal(err)
				}
			}
			if version != 2 || foreignKeys != 1 || messages != 1 || folders != 1 || triggers != 1 {
				t.Fatalf("升级后丢失数据、触发器或外键设置: %d %d %d %d %d", version, foreignKeys, messages, folders, triggers)
			}
			var workspaceID string
			if err := opened.DB.Raw("SELECT workspace_id FROM conversations WHERE title='保留会话'").Scan(&workspaceID).Error; err != nil || workspaceID != projectID {
				t.Fatalf("会话项目归属发生变化: %s %v", workspaceID, err)
			}
		})
	}
}

func TestConcurrentWorkspaceInitialization(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "empty"
		if legacy {
			name = "version_one"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data", "callum.db")
			if legacy {
				path, _, _ = versionOneProject(t)
			}
			errors := make(chan error, 4)
			var wait sync.WaitGroup
			for range 4 {
				wait.Go(func() {
					db, err := sqlite.Open(t.Context(), path)
					if err == nil {
						err = db.Close()
					}
					errors <- err
				})
			}
			wait.Wait()
			close(errors)
			for err := range errors {
				if err != nil {
					t.Error(err)
				}
			}
		})
	}
}
