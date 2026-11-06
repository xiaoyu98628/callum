package agent_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
)

func TestUserInputCancellationRaceAndRecovery(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		_, repository, ex, _ := fixture(t)
		now := time.Now().UnixMilli()
		in, err := repository.CreateUserInput(t.Context(), agent.Step{ID: newID(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: new(ex.Step.ID), ToolCallID: new("call-question"), Type: "user_input", Name: "问题", Status: agent.AwaitingUserInput, Input: json.RawMessage(`{"question":"请选择","options":["选项"]}`), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		if recovery {
			if err := repository.Recover(t.Context()); err != nil {
				t.Fatal(err)
			}
			step, err := repository.GetStep(t.Context(), in.ID)
			if err != nil || step.Status != agent.Interrupted {
				t.Fatal(step, err)
			}
			if _, _, err := repository.AnswerUserInput(t.Context(), in.ID, in.RunID, agent.InputAnswer{Text: "迟到回复"}); !errors.Is(err, agent.ErrConflict) {
				t.Fatal("中断后接受了回复", err)
			}
			continue
		}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		wg.Go(func() {
			_, _, err := repository.AnswerUserInput(t.Context(), in.ID, in.RunID, agent.InputAnswer{OptionIndex: new(0)})
			results <- err
		})
		wg.Go(func() { _, err := repository.CancelUserInput(t.Context(), in.ID); results <- err })
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil && !errors.Is(err, agent.ErrConflict) {
				t.Fatal(err)
			}
		}
		step, err := repository.GetStep(t.Context(), in.ID)
		if err != nil || step.Status != agent.Completed && step.Status != agent.Cancelled {
			t.Fatal(step, err)
		}
		value, err := agent.InputFromStep(step)
		if err != nil || value.Status == "pending" {
			t.Fatal(value, err)
		}
		if step.Status == agent.Cancelled && value.Answer != nil {
			t.Fatal("取消生成了虚假回复")
		}
	}
}
