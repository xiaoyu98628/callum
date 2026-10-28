package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

func ormTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestORMConnectionOwnershipAndContext(t *testing.T) {
	db := ormTestDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	orm, err := NewORM(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := orm.DB()
	if err != nil || pool != db || pool.Stats().MaxOpenConnections != 1 {
		t.Fatalf("ORM replaced the application pool or connection limit: %p, %v", pool, err)
	}
	cancel()
	var one int
	if err := orm.WithContext(t.Context()).Raw("SELECT 1").Scan(&one).Error; err != nil || one != 1 {
		t.Fatalf("startup context leaked into runtime queries: %d, %v", one, err)
	}
	var tables int64
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("ORM initialization unexpectedly changed schema: count=%d, error=%v", tables, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := orm.WithContext(t.Context()).Raw("SELECT 1").Scan(&one).Error; err == nil {
		t.Fatal("ORM opened a separate pool after the owner closed the database")
	}
}

func TestORMInitializationCancellationKeepsOwnerPool(t *testing.T) {
	db := ormTestDB(t)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := NewORM(ctx, db)
		result <- err
	}()
	var initErr error
	timedOut := false
	select {
	case initErr = <-result:
	case <-time.After(2 * time.Second):
		timedOut = true
	}
	// 即使初始化没有正确响应取消，也释放连接并等待测试任务，避免遗留 goroutine。
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if timedOut {
		initErr = <-result
		t.Errorf("ORM initialization ignored timeout while waiting for the pool: %v", initErr)
	}
	if !errors.Is(initErr, context.DeadlineExceeded) {
		t.Fatalf("initialization lost cancellation: %v", initErr)
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("failed ORM initialization closed the owner's pool: %v", err)
	}
}

func TestORMReconnectionPreservesSettings(t *testing.T) {
	db := ormTestDB(t)
	orm, err := NewORM(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	// 丢弃空闲物理连接，让 ORM 后续查询也走连接初始化的 DSN 配置。
	db.SetMaxIdleConns(0)
	for name, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000, "synchronous": 2} {
		var value int
		if err := orm.WithContext(t.Context()).Raw("PRAGMA " + name).Scan(&value).Error; err != nil || value != want {
			t.Fatalf("reconnected ORM query: %s=%d, want %d, error=%v", name, value, want, err)
		}
	}
	var mode string
	if err := orm.WithContext(t.Context()).Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil || mode != "wal" {
		t.Fatalf("ORM reconnection lost WAL: %s, %v", mode, err)
	}
}

func TestORMTransactionRollback(t *testing.T) {
	db := ormTestDB(t)
	if err := Migrate(t.Context(), db, discardMigrationLogger()); err != nil {
		t.Fatal(err)
	}
	orm, err := NewORM(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	rollbackErr := errors.New("rollback requested")
	err = orm.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		record := workspaceRecord{ID: "rollback", Name: "project", RootPath: t.TempDir(), CreatedAt: 1, UpdatedAt: 1}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("ORM transaction failed: %v", err)
	}
	var count int64
	if err := orm.WithContext(t.Context()).Model(&workspaceRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("transaction did not roll back or release the connection: count=%d, error=%v", count, err)
	}
}

func TestORMInvalidInitialization(t *testing.T) {
	db := ormTestDB(t)
	if _, err := NewORM(nil, db); err == nil {
		t.Fatal("accepted nil context")
	}
	if _, err := NewORM(t.Context(), nil); err == nil {
		t.Fatal("accepted nil pool")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewORM(ctx, db); !errors.Is(err, context.Canceled) {
		t.Fatalf("ignored cancelled initialization: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewORM(t.Context(), db); err == nil {
		t.Fatal("accepted closed database")
	}
}
