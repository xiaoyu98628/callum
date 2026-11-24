package agent_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestMessageSavePreservesCancellationAndSubsequentWrites(t *testing.T) {
	db, repository, execution, _ := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	const callback = "controlled_cancel_message_save"
	if err := db.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context != ctx {
			return
		}
		cancel()
		// 将隐式事务固定在已回滚状态，复现 database/sql 的取消回滚竞态。
		// 这与调度器先完成自动回滚的结果一致，不依赖随机等待。
		if transaction, ok := tx.Statement.ConnPool.(*sql.Tx); ok {
			if err := transaction.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Error(err)
			}
		}
		tx.AddError(ctx.Err())
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Update().Remove(callback); err != nil {
			t.Error(err)
		}
	})
	var before, after schema.Message
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	err := repository.SaveMessage(ctx, execution.Assistant.ID, []model.Block{{Type: "text", Text: "must not be written"}}, agent.Running)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误类型被回滚覆盖: %v", err)
	}
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before.Content, after.Content) || before.Status != after.Status {
		t.Fatal("取消的写入修改了原消息")
	}
	if err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "subsequent write"}}, agent.Running); err != nil {
		t.Fatalf("取消污染了后续写入: %v", err)
	}
}

func TestMessageSaveKeepsRealDatabaseErrors(t *testing.T) {
	db, repository, execution, _ := fixture(t)
	if err := db.DB.Exec(`CREATE TRIGGER reject_message_save BEFORE UPDATE ON conversation_messages BEGIN SELECT RAISE(ABORT,'controlled message write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "rejected"}}, agent.Running)
	if err == nil || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "controlled message write failure") {
		t.Fatalf("真实数据库错误被丢失: %v", err)
	}
	if err := db.DB.Exec("DROP TRIGGER reject_message_save").Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "after failed write"}}, agent.Running); err != nil {
		t.Fatal(err)
	}
}

func TestMessageSaveKeepsCallerTransaction(t *testing.T) {
	db, _, execution, _ := fixture(t)
	var before, after schema.Message
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("controlled caller rollback")
	err := db.DB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		repository, err := store.NewRepository(tx)
		if err != nil {
			return err
		}
		if err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "inside caller transaction"}}, agent.Running); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("调用方事务错误丢失: %v", err)
	}
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before.Content, after.Content) || before.Status != after.Status {
		t.Fatal("消息写入绕过了调用方事务")
	}
}

func TestStepQueriesReadMetadataAndUTF8Windows(t *testing.T) {
	db, repository, execution, toolStep := fixture(t)
	ctx := t.Context()
	content := "a🙂中\x00尾"
	output := `{"reasoning":"a🙂中\u0000尾","context_message":{"role":"assistant","content":"a🙂中\u0000尾"},"provider_extra":"` +
		strings.Repeat("源", 1<<18) + `"}`
	for _, id := range []string{execution.Step.ID, toolStep.ID} {
		// 模拟旧记录，JSON 仓储解码只去掉顶层前后的空白。
		if err := db.DB.Model(&schema.Step{}).Where("id=?", id).
			Updates(map[string]any{"output": "\t" + output + "\r\n", "usage": `{"total_tokens":5}`, "definition": `{"tools":[]}`}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, after := range []int64{0, 1} {
		summaries, err := repository.StepSummaries(ctx, execution.Run.ID, after, 1)
		if err != nil || len(summaries) != 1 || summaries[0].Sequence != after+1 {
			t.Fatalf("步骤游标错误: %+v %v", summaries, err)
		}
		summary := summaries[0]
		full, err := repository.GetStep(ctx, summary.ID)
		if err != nil || !reflect.DeepEqual(summary, agent.SummarizeStep(full)) || summary.OutputBytes != len(output) {
			t.Fatalf("列表元数据与旧查询不一致: %+v %v", summary, err)
		}
		single, err := repository.GetStepSummary(ctx, summary.ID)
		if err != nil || !reflect.DeepEqual(summary, single) {
			t.Fatalf("单个步骤的元数据与列表不一致: %+v %v", single, err)
		}
	}
	stepID := execution.Step.ID
	for _, tc := range []struct {
		field, want string
		offset      int
		next        int
		total       int
		present     bool
	}{
		{field: "output", want: output[:4], next: 4, total: len(output), present: true},
		{field: "reasoning", want: "a", next: 1, total: len(content), present: true},
		{field: "reasoning", want: "🙂", offset: 1, next: 5, total: len(content), present: true},
		{field: "reasoning", want: "中\x00", offset: 5, next: 9, total: len(content), present: true},
		{field: "reasoning", offset: len(content), next: len(content), total: len(content), present: true},
		{field: "text", want: "a", next: 1, total: len(content), present: true},
		{field: "text", want: "🙂", offset: 1, next: 5, total: len(content), present: true},
		{field: "text", want: "中\x00", offset: 5, next: 9, total: len(content), present: true},
		{field: "usage", want: `{"to`, next: 4, total: len(`{"total_tokens":5}`), present: true},
		{field: "definition", want: `{"to`, next: 4, total: len(`{"tools":[]}`), present: true},
	} {
		want := agent.StepData{Present: tc.present, Fragment: tc.want, TotalBytes: tc.total, NextOffset: tc.next}
		if got, err := repository.StepData(ctx, stepID, tc.field, tc.offset, 4); err != nil || got != want {
			t.Fatalf("%s at %d: %+v %v, want %+v", tc.field, tc.offset, got, err, want)
		}
	}
	for _, field := range []string{"reasoning", "text"} {
		for _, offset := range []int{2, len(content) + 1, math.MaxInt} {
			if _, err := repository.StepData(ctx, stepID, field, offset, 4); !errors.Is(err, agent.ErrInvalid) {
				t.Fatalf("%s 接受无效的字节偏移 %d: %v", field, offset, err)
			}
		}
		if got, err := repository.StepData(ctx, toolStep.ID, field, 0, 4); err != nil || got != (agent.StepData{}) {
			t.Fatalf("非模型步骤暴露了 %s: %+v %v", field, got, err)
		}
	}
	for _, tc := range []struct {
		field   string
		raw     any
		present bool
		err     error
	}{
		{field: "reasoning", raw: nil},
		{field: "reasoning", raw: `{"text":"旧记录"}`},
		{field: "reasoning", raw: `{"reasoning":null}`},
		{field: "reasoning", raw: `{"reasoning":""}`, present: true},
		{field: "reasoning", raw: `{"reasoning":23}`, err: agent.ErrStepReasoning},
		{field: "reasoning", raw: `[]`, err: agent.ErrStepReasoning},
		{field: "text", raw: nil},
		{field: "text", raw: `{"text":"旧记录"}`},
		{field: "text", raw: `{"context_message":{"role":"assistant","content":null}}`},
		{field: "text", raw: `{"context_message":{"role":"assistant","content":""}}`, present: true},
		{field: "text", raw: `{"context_message":{"role":"assistant","content":23}}`},
		{field: "text", raw: `{"context_message":{"role":"tool","content":"工具结果"}}`},
	} {
		if err := db.DB.Model(&schema.Step{}).Where("id=?", stepID).Update("output", tc.raw).Error; err != nil {
			t.Fatal(err)
		}
		got, err := repository.StepData(ctx, stepID, tc.field, 0, 4)
		if !errors.Is(err, tc.err) || got != (agent.StepData{Present: tc.present}) {
			t.Fatalf("%s 字段 %v: %+v %v", tc.field, tc.raw, got, err)
		}
	}
}
