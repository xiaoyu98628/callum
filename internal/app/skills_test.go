package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
)

func TestBundledOfficeSkillsThroughAgentAndFollowUp(t *testing.T) {
	t.Parallel()
	runtimeDirectory := os.Getenv("CALLUM_TEST_RUNTIME_DIR")
	skillDirectory := os.Getenv("CALLUM_TEST_SKILLS_DIR")
	if runtimeDirectory == "" || skillDirectory == "" {
		t.Skip("真实技能验证需要 CALLUM_TEST_RUNTIME_DIR 与客户端 CALLUM_TEST_SKILLS_DIR")
	}
	python := filepath.Join(runtimeDirectory, "python", "bin", "python3")
	if os.PathSeparator == '\\' {
		python = filepath.Join(runtimeDirectory, "python", "python.exe")
	}
	fixtures := t.TempDir()
	create := exec.CommandContext(t.Context(), python, "-I", "-B", "-c", `
from pathlib import Path
import sys
from docx import Document
from openpyxl import Workbook
from pptx import Presentation
p=Path(sys.argv[1])
d=Document(); d.add_paragraph('office-fixture-42'); d.save(p/'report.docx')
w=Workbook(); w.active['A1']='office-fixture-42'; w.save(p/'report.xlsx'); w.close()
r=Presentation(); s=r.slides.add_slide(r.slide_layouts[1]); s.shapes.title.text='office-fixture-42'; r.save(p/'report.pptx')
`, fixtures)
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("创建真实 Office 样本: %v %s", err, output)
	}
	for _, sample := range []struct{ skill, extension, reader string }{{"documents", "docx", "from docx import Document; import sys; print([p.text for p in Document(sys.argv[1]).paragraphs])"}, {"spreadsheets", "xlsx", "from openpyxl import load_workbook; import sys; w=load_workbook(sys.argv[1],read_only=True); print(w.active['A1'].value); w.close()"}, {"presentations", "pptx", "from pptx import Presentation; import sys; print(Presentation(sys.argv[1]).slides[0].shapes.title.text)"}} {
		t.Run(sample.skill, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.RuntimeDir = runtimeDirectory
			cfg.MaxConcurrentRuns = 1
			if err := os.MkdirAll(filepath.Join(cfg.DataDir, "skills"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.CopyFS(filepath.Join(cfg.DataDir, "skills"), os.DirFS(skillDirectory)); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			var lastWork string
			reference := regexp.MustCompile(`[0-9a-f-]{36}/[0-9a-f-]{36}/[0-9a-f]{64}\.[a-z0-9]+`)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				var request struct {
					Messages []struct {
						Role    string          `json:"role"`
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
				}
				if err := json.Unmarshal(data, &request); err != nil {
					t.Error(err)
				}
				number := int(calls.Add(1))
				if number > 4 && !strings.Contains(string(data), "followup-skill") {
					t.Error("新增技能未进入后续请求")
				}
				var toolContent, skillContext string
				for _, message := range request.Messages {
					if message.Role == "tool" {
						_ = json.Unmarshal(message.Content, &toolContent)
					}
					if message.Role == "system" {
						var content string
						if err := json.Unmarshal(message.Content, &content); err != nil {
							t.Error(err)
						}
						skillContext += content
					}
				}
				toolName := ""
				var arguments any
				switch (number - 1) % 4 {
				case 0:
					toolName, arguments = "read_file", map[string]any{"path": filepath.Join(cfg.DataDir, "skills", ".system", sample.skill, "SKILL.md")}
					if !strings.Contains(string(data), "Codex SKILL.md") {
						t.Error("模型未收到技能目录")
					}
					if !strings.Contains(skillContext, filepath.Join(cfg.DataDir, "skills", ".system", sample.skill, "SKILL.md")) {
						t.Error("当前文件类型的技能未进入目录")
					}
				case 1:
					toolName, arguments = "open_attachment", map[string]any{"attachment_ref": reference.FindString(string(data))}
					if !strings.Contains(toolContent, "Callum") {
						t.Error("技能正文未通过工具读取")
					}
				case 2:
					var value struct {
						Result struct {
							Path string `json:"path"`
							Work string `json:"work_directory"`
						} `json:"result"`
					}
					if err := json.Unmarshal([]byte(toolContent), &value); err != nil {
						t.Error(err)
					}
					if value.Result.Path == "" || value.Result.Work == "" {
						t.Errorf("未得到附件副本: %s", toolContent)
					}
					lastWork = value.Result.Work
					toolName, arguments = "run_command", map[string]any{"command": []string{"python3", "-c", sample.reader, value.Result.Path}, "cwd": value.Result.Work}
				case 3:
					if !strings.Contains(toolContent, "office-fixture-42") {
						t.Errorf("模型未收到真实文档内容: %s", toolContent)
					}
				}
				message := map[string]any{"role": "assistant", "content": "文档已读取"}
				finish := "stop"
				if toolName != "" {
					raw, err := json.Marshal(arguments)
					if err != nil {
						t.Error(err)
					}
					message["tool_calls"] = []any{map[string]any{"id": "call_" + strconv.Itoa(number), "type": "function", "function": map[string]any{"name": toolName, "arguments": string(raw)}}}
					finish = "tool_calls"
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
			}))
			defer upstream.Close()
			runtime, err := newTestRuntime(t.Context(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := runtime.Close(); err != nil {
					t.Error(err)
				}
			})
			p := sharedPeer(t, runtime)
			providerID := resultString(t, p.call(t, "provider.create", map[string]any{"name": "Office", "protocol": "chatcompletions", "base_url": upstream.URL + "/v1"}), "id")
			modelID := resultString(t, p.call(t, "model.create", map[string]any{"provider_id": providerID, "display_name": "Office", "remote_model_id": "office", "enabled": true, "context_window": 1000000, "model_type": []string{"text"}}), "id")
			first := p.call(t, "conversation.send", map[string]any{"model_id": modelID, "stream": false, "content": []map[string]string{{"type": "file", "path": filepath.Join(fixtures, "report."+sample.extension), "name": "report." + sample.extension}}})
			conv := resultString(t, first, "conversation_id")
			p.event(t, "run.completed")
			if _, err := os.Stat(lastWork); !os.IsNotExist(err) {
				t.Fatalf("执行结束后工作目录未清理: %v", err)
			}
			extra := filepath.Join(cfg.DataDir, "skills", "followup")
			if err := os.Mkdir(extra, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(extra, "SKILL.md"), []byte("---\nname: followup-skill\ndescription: user added skill\n---\n"), 0600); err != nil {
				t.Fatal(err)
			}
			requireOK(t, p.call(t, "conversation.send", map[string]any{"conversation_id": conv, "stream": false, "content": []map[string]string{{"type": "text", "text": fmt.Sprintf("继续读取 %s", sample.skill)}}}))
			p.event(t, "run.completed")
			if calls.Load() != 8 {
				t.Fatalf("工具循环未完整执行: %d", calls.Load())
			}
			p.stop(t)
		})
	}
}
