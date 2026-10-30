package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/config"
)

type wireFrame struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
type appPeer struct {
	conn     net.Conn
	decoder  *json.Decoder
	done     chan error
	cancel   context.CancelFunc
	pending  []wireFrame
	sequence int
	stopped  bool
	seen     strings.Builder
}

func startApp(t *testing.T, cfg config.Config, logs io.Writer) *appPeer {
	t.Helper()
	server, client := net.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	p := &appPeer{conn: client, decoder: json.NewDecoder(client), done: make(chan error, 1), cancel: cancel}
	go func() { p.done <- Run(ctx, cfg, server, server, slog.New(slog.NewJSONHandler(logs, nil))) }()
	t.Cleanup(func() {
		cancel()
		client.Close()
		if !p.stopped {
			select {
			case err := <-p.done:
				if err != nil {
					t.Errorf("应用未正常清理: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Error("应用未退出")
			}
		}
	})
	if f := p.read(t); f.Method != "system.ready" {
		t.Fatalf("应用未就绪: %+v", f)
	}
	return p
}
func (p *appPeer) read(t *testing.T) wireFrame {
	t.Helper()
	if err := p.conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var f wireFrame
	if err := p.decoder.Decode(&f); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	p.seen.Write(encoded)
	return f
}
func (p *appPeer) call(t *testing.T, method string, params any) wireFrame {
	t.Helper()
	p.sequence++
	id := strconv.Itoa(p.sequence)
	if err := p.conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(p.conn).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
	for {
		f := p.read(t)
		if f.ID == id {
			return f
		}
		p.pending = append(p.pending, f)
	}
}
func (p *appPeer) event(t *testing.T, method string) wireFrame {
	t.Helper()
	for i, f := range p.pending {
		if f.Method == method {
			p.pending = append(p.pending[:i], p.pending[i+1:]...)
			return f
		}
	}
	for {
		f := p.read(t)
		if f.Method == method {
			return f
		}
		p.pending = append(p.pending, f)
	}
}
func (p *appPeer) stop(t *testing.T) {
	t.Helper()
	f := p.call(t, "system.shutdown", map[string]any{})
	requireOK(t, f)
	select {
	case err := <-p.done:
		p.stopped = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("关闭超时")
	}
}
func requireOK(t *testing.T, f wireFrame) {
	t.Helper()
	if f.Error != nil {
		t.Fatalf("接口失败: %+v", f.Error)
	}
}
func resultID(t *testing.T, f wireFrame, key string) string {
	t.Helper()
	requireOK(t, f)
	var result map[string]string
	if err := json.Unmarshal(f.Result, &result); err != nil || result[key] == "" {
		t.Fatalf("缺少标识: %s %v", f.Result, err)
	}
	return result[key]
}

func TestStorageToModelThroughStdio(t *testing.T) {
	cancelObserved := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fake-integration-key" {
			t.Error("端点或运行凭据不正确")
			w.WriteHeader(400)
			return
		}
		var request struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "remote-test-model" || len(request.Messages) == 0 {
			t.Errorf("请求转换错误: %v", err)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("X-Request-ID", "upstream-request")
		if request.Messages[0].Content == "供应商失败" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := io.WriteString(w, `{"error":{"message":"fake-integration-key","type":"authentication_error"}}`); err != nil {
				t.Error(err)
			}
			return
		}
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(w, `{"id":"reply","choices":[{"index":0,"message":{"role":"assistant","content":"完整回答"},"finish_reason":"stop"}]}`); err != nil {
				t.Error(err)
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"流式回答\"},\"finish_reason\":null}]}\n\n"); err != nil {
			t.Error(err)
			return
		}
		w.(http.Flusher).Flush()
		if request.Messages[0].Content == "等待取消" {
			<-r.Context().Done()
			cancelObserved <- struct{}{}
			return
		}
		if _, err := io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	p := startApp(t, cfg, &logs)
	pid := resultID(t, p.call(t, "provider.create", map[string]any{"name": "集成厂商", "protocol": "chatcompletions", "base_url": upstream.URL + "/v1", "api_key": "fake-integration-key"}), "id")
	modelParams := map[string]any{"provider_id": pid, "display_name": "集成模型", "remote_model_id": "remote-test-model", "enabled": true}
	mid := resultID(t, p.call(t, "model.create", modelParams), "id")
	invoke := func(stream bool, input string) string {
		return resultID(t, p.call(t, "model.invoke", map[string]any{"model_id": mid, "stream": stream, "messages": []map[string]string{{"role": "user", "content": input}}}), "run_id")
	}
	run := invoke(false, "非流式")
	f := p.event(t, "run.completed")
	if !bytes.Contains(f.Params, []byte(run)) || !bytes.Contains(f.Params, []byte("完整回答")) || !bytes.Contains(f.Params, []byte("upstream-request")) {
		t.Fatalf("非流式结果错误: %s", f.Params)
	}
	invoke(true, "流式")
	p.event(t, "run.text_delta")
	p.event(t, "run.completed")
	invoke(false, "供应商失败")
	f = p.event(t, "run.failed")
	if !bytes.Contains(f.Params, []byte(`"http_status":401`)) || !bytes.Contains(f.Params, []byte("upstream-request")) {
		t.Fatalf("供应商错误信息丢失: %s", f.Params)
	}
	first := invoke(true, "等待取消")
	p.event(t, "run.text_delta")
	second := invoke(false, "另一调用")
	f = p.event(t, "run.completed")
	if !bytes.Contains(f.Params, []byte(second)) {
		t.Fatalf("执行隔离错误: %s", f.Params)
	}
	requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": first}))
	f = p.event(t, "run.cancelled")
	if !bytes.Contains(f.Params, []byte(first)) {
		t.Fatal("取消了错误的执行")
	}
	select {
	case <-cancelObserved:
	case <-time.After(3 * time.Second):
		t.Fatal("取消没有传递到 HTTP 请求")
	}
	requireOK(t, p.call(t, "model.set_enabled", map[string]any{"id": mid, "enabled": false}))
	f = p.call(t, "model.invoke", map[string]any{"model_id": mid, "messages": []map[string]string{{"role": "user", "content": "禁用"}}})
	if f.Error == nil || f.Error.Code != 1004 {
		t.Fatalf("禁用模型仍然接受调用: %+v", f)
	}
	requireOK(t, p.call(t, "model.set_enabled", map[string]any{"id": mid, "enabled": true}))
	modelParams["id"] = mid
	modelParams["capabilities"] = map[string]bool{"tools": true}
	modelParams["model_type"] = []string{"tools"}
	requireOK(t, p.call(t, "model.update", modelParams))
	f = p.call(t, "model.invoke", map[string]any{"model_id": mid, "messages": []map[string]string{{"role": "user", "content": "工具"}}})
	if f.Error == nil || f.Error.Code != 1005 {
		t.Fatalf("未实现的功能被静默忽略: %+v", f)
	}
	p.stop(t)
	if strings.Contains(p.seen.String(), "fake-integration-key") || strings.Contains(logs.String(), "fake-integration-key") {
		t.Fatal("凭据泄露到协议或日志")
	}
	// 重启复用同一临时数据库，检查持久化的 UUID 不发生变化。
	p2 := startApp(t, cfg, io.Discard)
	f = p2.call(t, "model.list", map[string]any{})
	requireOK(t, f)
	if !bytes.Contains(f.Result, []byte(mid)) || !bytes.Contains(f.Result, []byte(pid)) {
		t.Fatalf("重启后配置丢失: %s", f.Result)
	}
	p2.stop(t)
}

type ownedInput struct {
	io.Reader
	closed int
	err    error
}

func (c *ownedInput) Close() error { c.closed++; return c.err }

type ownedOutput struct {
	bytes.Buffer
	closed int
	err    error
}

func (c *ownedOutput) Close() error { c.closed++; return c.err }
func TestInitializationFailureClosesOwnedStreams(t *testing.T) {
	closeErr := errors.New("测试关闭错误")
	in := &ownedInput{Reader: strings.NewReader(""), err: closeErr}
	out := &ownedOutput{}
	cfg := config.Config{DatabasePath: "relative.db", MaxConcurrentRuns: 1, CallTimeout: time.Second}
	err := Run(t.Context(), cfg, in, out, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if !errors.Is(err, closeErr) || in.closed != 1 || out.closed != 1 || out.Len() != 0 {
		t.Fatalf("初始化失败清理错误: %v %d %d", err, in.closed, out.closed)
	}
}

func TestBoundedUpstreamBody(t *testing.T) {
	for _, length := range []int{4, 5} {
		body := &boundedBody{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat("x", length))), remaining: 4}
		data, err := io.ReadAll(body)
		if string(data) != "xxxx" || (length == 5 && !errors.Is(err, errResponseLimit)) || (length == 4 && err != nil) {
			t.Fatalf("响应边界错误: %q %v", data, err)
		}
		if err := body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
