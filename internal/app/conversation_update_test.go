package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

func TestConversationUpdateUsesResolvedModelAndPreservesProtocol(t *testing.T) {
	runtime, _ := runtimeFixture(t, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"回答"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	firstModel := directModel(t, runtime, upstream.URL)
	secondModel := directModel(t, runtime, upstream.URL)
	disabled := directModel(t, runtime, upstream.URL)
	if err := runtime.Providers().SetModelEnabled(t.Context(), disabled, false); err != nil {
		t.Fatal(err)
	}
	sub, err := runtime.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	var accepted conversation.Execution
	if err := runtime.Conversations().Send(t.Context(), conversation.Send{ModelID: firstModel, Stream: new(false), Content: []model.Block{{Type: "text", Text: "问题"}}}, func(ex conversation.Execution) error {
		accepted = ex
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	nextBusiness(t, sub, "run.completed", accepted.Run.ID)
	id := accepted.Conversation.ID
	before, err := runtime.Conversations().Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, modelID string
		want          error
	}{
		{"invalid", "invalid", conversation.ErrInvalid},
		{"missing", policyTestID(t), provider.ErrNotFound},
		{"disabled", disabled, provider.ErrDisabled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runtime.Conversations().Update(t.Context(), id, conversation.Update{Title: new("不应保存"), Pinned: new(true), ModelID: tt.modelID})
			if !errors.Is(err, tt.want) {
				t.Fatalf("错误模型未被拒绝: %v", err)
			}
			after, err := runtime.Conversations().Get(t.Context(), id)
			if err != nil || after.Model != before.Model || after.Title != before.Title || after.PinnedAt != nil || after.UpdatedAt != before.UpdatedAt {
				t.Fatalf("失败更新发生了部分写入: %+v %v", after, err)
			}
		})
	}
	updated, err := runtime.Conversations().Update(t.Context(), id, conversation.Update{Title: new("  人工标题  "), Pinned: new(true)})
	if err != nil || updated.Title != "人工标题" || updated.TitleSource != "manual" || updated.PinnedAt == nil || updated.Model != before.Model {
		t.Fatalf("普通更新丢失字段或改写模型: %+v %v", updated, err)
	}
	updated, err = runtime.Conversations().Update(t.Context(), id, conversation.Update{ModelID: secondModel})
	if err != nil || updated.Model.ID != secondModel || updated.Model.RemoteModelID != "remote" || updated.Model.Provider.ID == before.Model.Provider.ID {
		t.Fatalf("未使用新模型的服务端快照: %+v %v", updated, err)
	}
	peer := sharedPeer(t, runtime)
	for _, field := range []string{"model", "deleted"} {
		frame := peer.call(t, "conversation.update", map[string]any{"id": id, field: map[string]string{"id": firstModel}})
		if frame.Error == nil || frame.Error.Code != -32602 {
			t.Fatalf("协议接受内部写入字段 %s: %+v", field, frame)
		}
	}
	frame := peer.call(t, "conversation.update", map[string]any{"id": id, "title": "协议标题", "model_id": firstModel, "pinned": false, "archived": true})
	requireOK(t, frame)
	var wire conversation.Conversation
	if err := json.Unmarshal(frame.Result, &wire); err != nil || wire.ID != id || wire.Title != "协议标题" || wire.Model != before.Model || wire.PinnedAt != nil || wire.ArchivedAt == nil {
		t.Fatalf("更新协议不兼容: %s %v", frame.Result, err)
	}
	for _, method := range []string{"conversation.delete", "conversation.restore"} {
		frame := peer.call(t, method, map[string]string{"id": id})
		requireOK(t, frame)
		if err := json.Unmarshal(frame.Result, &wire); err != nil || (wire.DeletedAt != nil) != (method == "conversation.delete") {
			t.Fatalf("删除或恢复不兼容: %s %v", frame.Result, err)
		}
	}
}
