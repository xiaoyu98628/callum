package agent

import (
	"encoding/json"
	"testing"
)

func TestQuestionAndAnswerBoundaries(t *testing.T) {
	for _, invalid := range []string{
		`{}`, `null`, `{"question":"问题","Question":"重复","options":[]}`, `{"question":"问题","question":"重复","options":[]}`,
		`{"question":"问题","options":null}`, `{"question":"问题","options":["相同"," 相同 "]}`, `{"question":"问题","options":[],"approved":true}`,
		`{"question":"问题","options":["1","2","3","4","5","6","7","8","9"]}`,
	} {
		if _, err := ParseInputQuestion([]byte(invalid)); err == nil {
			t.Fatalf("接受无效问题 %s", invalid)
		}
	}
	question, err := ParseInputQuestion([]byte(`{"question":" 请确认 ","options":["已完成","进行中"]}`))
	if err != nil || question.Question != "请确认" {
		t.Fatal(question, err)
	}
	for _, invalid := range []InputAnswer{{}, {OptionIndex: new(-1)}, {OptionIndex: new(2)}, {Text: " \n"}, {Skipped: true, Text: "文字"}, {Skipped: true, OptionIndex: new(0)}} {
		if ValidateInputAnswer(question, invalid) == nil {
			t.Fatalf("接受无效回复 %+v", invalid)
		}
	}
	for _, answer := range []InputAnswer{{OptionIndex: new(0)}, {Text: "自己的回复"}, {Skipped: true}, {OptionIndex: new(1), Text: "补充说明"}} {
		if err := ValidateInputAnswer(question, answer); err != nil {
			t.Fatal(err)
		}
		input, _ := json.Marshal(question)
		step := Step{ID: "question", RunID: "run", Type: "user_input", Status: Completed, ToolCallID: new("call"), Input: input}
		step.Output, err = InputAnswerOutput(step, answer)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := InputFromStep(step)
		if err != nil || parsed.Answer == nil || parsed.Answer.Text != answer.Text {
			t.Fatal(parsed, err)
		}
	}
}
