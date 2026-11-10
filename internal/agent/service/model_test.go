package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/xiaoyu98628/callum/internal/inference"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
)

type interruptedMessageRepository struct {
	agent.Repository
	write        func(context.Context) error
	cleanupError error
}

func (r *interruptedMessageRepository) SaveMessage(ctx context.Context, _ string, _ []model.Block, _ string) error {
	return r.write(ctx)
}

func (r *interruptedMessageRepository) SaveStep(_ context.Context, step agent.Step) error {
	if step.Status != agent.Running {
		return r.cleanupError
	}
	return nil
}

type preambleStream struct{ model.Client }

func (preambleStream) Stream(_ context.Context, _ model.Request, emit func(model.Event) error) error {
	return emit(model.Event{Type: model.EventTextDelta, Text: "准备检查项目。"})
}

func TestCancelledIncrementalSaveDoesNotBecomeStorageFailure(t *testing.T) {
	writeFailure := errors.New("controlled storage failure")
	for _, test := range []struct {
		name                           string
		cancel, failWrite, failCleanup bool
		wantStatus                     string
		wantStorage                    bool
	}{
		{"cancelled_write", true, false, false, agent.Cancelled, false},
		{"real_write_failure", false, true, false, agent.Failed, true},
		{"real_write_failure_with_cancel", true, true, false, agent.Failed, true},
		{"cancelled_write_with_cleanup_failure", true, false, true, agent.Failed, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writes := 0
			repository := &interruptedMessageRepository{write: func(ctx context.Context) error {
				writes++
				// 确定性地让取消发生在保存正文时，不依赖网络与数据库的调度时机。
				if test.cancel {
					cancel()
				}
				if test.failWrite {
					return writeFailure
				}
				return ctx.Err()
			}}
			if test.failCleanup {
				repository.cleanupError = writeFailure
			}
			manager, err := executor.New(t.Context(), 1, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			engine := &Engine{repository: repository, manager: manager, emit: func(agent.Event) error { return nil }, options: Options{CallTimeout: time.Second}}
			ex := conversation.Execution{Run: agent.Run{Stream: true}, Assistant: conversation.Message{ID: "assistant"}}
			resolved := agent.Resolved{Client: preambleStream{}}
			total := 0
			_, err = engine.callModel(ctx, ex, resolved, agent.Step{Input: []byte(`{}`)}, nil, nil, func() {}, &total)
			status, _, _ := failure(err)
			if writes != 1 || status != test.wantStatus || errors.Is(err, agent.ErrStorage) != test.wantStorage {
				t.Fatalf("增量保存错误分类不符: writes=%d status=%s err=%v", writes, status, err)
			}
			if test.cancel && !errors.Is(err, context.Canceled) {
				t.Fatal("取消原因丢失")
			}
			if (test.failWrite || test.failCleanup) && !errors.Is(err, writeFailure) {
				t.Fatal("真实存储错误链丢失")
			}
		})
	}
}

// 测试同时覆盖供应商未交付最终结果的取消，以及超过累计内容边界的分片。
type reasoningStream struct {
	model.Client
	cancel bool
	reject bool
}

func (c reasoningStream) Stream(ctx context.Context, _ model.Request, emit func(model.Event) error) error {
	if err := emit(model.Event{Type: model.EventReasoningDelta, Text: "已收到的推理"}); err != nil {
		return err
	}
	if c.reject {
		return emit(model.Event{Type: model.EventReasoningDelta, Text: strings.Repeat("x", inference.MaxTextBytes)})
	}
	if c.cancel {
		return context.Canceled
	}
	return nil
}

type reasoningRepository struct {
	agent.Repository
	final   agent.Step
	partial bool
}

func (r *reasoningRepository) SaveStep(_ context.Context, step agent.Step) error {
	if step.Status == agent.Running && len(step.Output) > 0 {
		r.partial = true
	}
	if step.Status != agent.Running {
		r.final = step
	}
	return nil
}
func TestReasoningPersistsOnCancellationAndLimit(t *testing.T) {
	for _, reject := range []bool{false, true} {
		repo := &reasoningRepository{}
		manager, err := executor.New(t.Context(), 1, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(manager.Close)
		count := 0
		engine := &Engine{repository: repo, manager: manager, options: Options{CallTimeout: time.Second}, emit: func(event agent.Event) error {
			if event.Type == "reasoning.delta" {
				count++
			}
			return nil
		}}
		total := 0
		result, err := engine.callModel(t.Context(), conversation.Execution{Run: agent.Run{Stream: true}}, agent.Resolved{Client: reasoningStream{cancel: !reject, reject: reject}}, agent.Step{Input: []byte(`{}`)}, nil, nil, func() {}, &total)
		want := context.Canceled
		if reject {
			want = agent.ErrOutputLimit
		}
		if !errors.Is(err, want) || count != 1 || !repo.partial {
			t.Fatalf("推理取消或限额行为错误: count=%d partial=%v err=%v", count, repo.partial, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(repo.final.Output, &fields); err != nil {
			t.Fatal(err)
		}
		var text string
		if err := json.Unmarshal(fields["reasoning"], &text); err != nil {
			t.Fatal(err)
		}
		if text != "已收到的推理" || result.Reasoning == nil || *result.Reasoning != text || fields["context_message"] != nil {
			t.Fatalf("部分推理保存错误: %s", repo.final.Output)
		}
	}
}

func TestActualInputProjectionKeepsUnknownAndOriginalOutput(t *testing.T) {
	for _, input := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`{"provider_result":{"text":"原始结果"}}`)} {
		output, err := withActualInputTokens(input, nil)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]json.RawMessage
		if err := json.Unmarshal(output, &value); err != nil {
			t.Fatal(err)
		}
		if string(value["context_usage"]) != `{"actual_input_tokens":null}` {
			t.Fatal("缺失实际用量被补造")
		}
		if len(input) > 4 && string(value["provider_result"]) != `{"text":"原始结果"}` {
			t.Fatal("原始输出被改写")
		}
	}
	output, err := withActualInputTokens(nil, new(int64(42)))
	if err != nil || string(output) != `{"context_usage":{"actual_input_tokens":42}}` {
		t.Fatalf("实际输入投影丢失: %s %v", output, err)
	}
}
