package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
)

func TestOversizedArgumentsCorrectBeforeExecutingBatch(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			root := t.TempDir()
			never, first, second := filepath.Join(root, "never.txt"), filepath.Join(root, "one.txt"), filepath.Join(root, "two.txt")
			patch := func(path, text string) string {
				return correctionJSON(t, map[string]any{"patch": "*** Begin Patch\n*** Add File: " + filepath.ToSlash(path) + "\n+" + text + "\n*** End Patch\n", "base_versions": []any{}})
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Stream   bool
					Tools    []json.RawMessage
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(input.Tools) == 0 {
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "title"}, "stop")
					return
				}
				n := calls.Add(1)
				if n == 1 {
					message := map[string]any{"content": "准备生成全文文件", "tool_calls": []any{
						correctionCall("valid-but-not-executed", "apply_patch", patch(never, "never"), input.Stream, 0),
						correctionCall("large", "apply_patch", patch(filepath.Join(root, "huge.txt"), strings.Repeat("中", 5500)), input.Stream, 1),
					}}
					if input.Stream {
						// 复现实际故障，参数超限时尚未收到 finish_reason，关闭读取后才结束发送。
						w.Header().Set("Content-Type", "text/event-stream")
						chunk := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": message}}}
						_, _ = fmt.Fprintf(w, "data: %s\n\n", correctionJSON(t, chunk))
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					} else {
						writeFileToolResponse(t, w, false, message, "tool_calls")
					}
					return
				}
				if n == 2 {
					feedback := ""
					for _, message := range input.Messages {
						if strings.Contains(message.Content, "服务端工具参数校验反馈") {
							feedback = message.Content
						}
					}
					if !strings.Contains(feedback, "16384") || !strings.Contains(feedback, "apply_patch") || !strings.Contains(feedback, "均未执行") {
						t.Errorf("缺少可操作的超限反馈: %q", feedback)
					}
					if _, err := os.Stat(never); !os.IsNotExist(err) {
						t.Error("执行了超限批次中的有效调用")
					}
					writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{correctionCall("one", "apply_patch", patch(first, "one"), input.Stream, 0), correctionCall("two", "apply_patch", patch(second, "two"), input.Stream, 1)}}, "tool_calls")
					return
				}
				writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "done"}, "stop")
			}))
			defer upstream.Close()
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, upstream.URL)
			wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "bounded", "root_path": root}), "id")
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": stream, "content": messageBlocks("生成文件")})
			rid := resultString(t, accepted, "run_id")
			p.event(t, "run.completed")
			for path, want := range map[string]string{first: "one\n", second: "two\n"} {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != want {
					t.Fatalf("纠正后的文件缺失: %q %v", data, err)
				}
			}
			if _, err := os.Stat(never); !os.IsNotExist(err) {
				t.Fatal("坏批次被执行")
			}
			failed, tools := 0, 0
			for _, step := range correctionSteps(t, p, rid) {
				if step.Type == "tool_call" {
					tools++
				}
				if step.Type == "model_call" && step.Status == agent.Failed {
					failed++
					if strings.Contains(correctionData(t, p, step.ID, "output"), "context_message") {
						t.Fatal("超限片段进入可恢复模型历史")
					}
				}
			}
			if failed != 1 || tools != 2 || calls.Load() != 3 {
				t.Fatalf("纠正调用次数异常: failed=%d tools=%d calls=%d", failed, tools, calls.Load())
			}
		})
	}
}
