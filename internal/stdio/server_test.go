package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

type testClient struct{}

func (testClient) Generate(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: "完整回答", RequestID: "provider-id"}, nil
}
func (testClient) Stream(_ context.Context, _ model.Request, emit func(model.Event) error) error {
	if err := emit(model.Event{Type: model.EventTextDelta, Text: "流式回答"}); err != nil {
		return err
	}
	return emit(model.Event{Type: model.EventCompleted, RequestID: "provider-id"})
}

type frame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Params json.RawMessage `json:"params"`
	Error  *rpcError       `json:"error"`
}
type peer struct {
	input      *io.PipeWriter
	output     *io.PipeReader
	frames     chan frame
	done       chan error
	readerDone chan struct{}
	cancel     context.CancelFunc
}

func storeForTest(t *testing.T) *provider.Service {
	t.Helper()
	service, err := provider.NewService(&memoryRepository{providers: make(map[string]provider.ProviderInput), models: make(map[string]provider.Model)})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type serverFixture struct {
	providers    *provider.Service
	resolve      inference.Resolver
	writeTimeout time.Duration
}

func serverForTest(t *testing.T, providers *provider.Service) *serverFixture {
	t.Helper()
	return &serverFixture{providers: providers, resolve: func(context.Context, string) (model.Client, string, error) { return testClient{}, "remote", nil }, writeTimeout: defaultWriteTimeout}
}

// 测试装配独立的连接和执行管理器，遵循应用中的关闭顺序。
func (f *serverFixture) Serve(ctx context.Context, in io.ReadCloser, out io.WriteCloser) (retErr error) {
	connection, err := NewConnection(ctx, in, out)
	if err != nil {
		return err
	}
	connection.writeTimeout = f.writeTimeout
	defer func() { retErr = errors.Join(retErr, connection.Close()) }()
	runs, err := inference.New(connection.Context(), f.resolve, 2, time.Second, connection.Emit)
	if err != nil {
		return err
	}
	defer runs.Close()
	server, err := New(f.providers, runs, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		return err
	}
	return server.Serve(connection)
}
func openPeer(t *testing.T, server *serverFixture) *peer {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	p := &peer{input: inW, output: outR, frames: make(chan frame, 32), done: make(chan error, 1), readerDone: make(chan struct{}), cancel: cancel}
	go func() { p.done <- server.Serve(ctx, inR, outW) }()
	go func() {
		defer close(p.readerDone)
		defer close(p.frames)
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 4096), maxResponseBytes+2)
		for scanner.Scan() {
			var f frame
			if json.Unmarshal(scanner.Bytes(), &f) != nil {
				return
			}
			select {
			case p.frames <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		inW.Close()
		outR.Close()
		select {
		case err := <-p.done:
			if err != nil {
				t.Errorf("服务清理失败: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("服务未退出")
		}
		select {
		case <-p.readerDone:
		case <-time.After(3 * time.Second):
			t.Error("协议读取未退出")
		}
	})
	if f := p.next(t); f.Method != "system.ready" {
		t.Fatalf("缺少就绪事件: %+v", f)
	}
	return p
}
func (p *peer) next(t *testing.T) frame {
	t.Helper()
	select {
	case f, ok := <-p.frames:
		if !ok {
			t.Fatal("协议输出提前关闭")
		}
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("等待协议消息超时")
		return frame{}
	}
}
func (p *peer) raw(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(p.input, line+"\n"); err != nil {
		t.Fatal(err)
	}
}
func (p *peer) call(t *testing.T, method string, params any) frame {
	t.Helper()
	data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "request", "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	p.raw(t, string(data))
	f := p.next(t)
	if string(f.ID) != `"request"` {
		t.Fatalf("请求响应顺序错误: %+v", f)
	}
	return f
}
func assertOK(t *testing.T, f frame) {
	t.Helper()
	if f.Error != nil {
		t.Fatalf("请求失败: %+v", f.Error)
	}
}

func TestCatalogThroughProtocol(t *testing.T) {
	store := storeForTest(t)
	p := openPeer(t, serverForTest(t, store))
	providerParams := map[string]any{"name": "测试厂商", "protocol": "chatcompletions", "base_url": "https://example.invalid/v1", "api_key": "fake-private-key"}
	f := p.call(t, "provider.create", providerParams)
	assertOK(t, f)
	var providerID idResult
	if err := json.Unmarshal(f.Result, &providerID); err != nil {
		t.Fatal(err)
	}
	providerParams["id"] = providerID.ID
	providerParams["name"] = "更新名称"
	delete(providerParams, "api_key")
	assertOK(t, p.call(t, "provider.update", providerParams))
	f = p.call(t, "provider.list", map[string]any{})
	assertOK(t, f)
	if bytes.Contains(f.Result, []byte("fake-private-key")) || !bytes.Contains(f.Result, []byte(`"has_api_key":true`)) {
		t.Fatalf("凭据查询不正确: %s", f.Result)
	}
	params := map[string]any{"provider_id": providerID.ID, "display_name": "测试模型", "remote_model_id": "remote", "enabled": true, "capabilities": map[string]bool{"reasoning": true}, "model_type": []string{"reasoning"}, "reasoning_levels": []map[string]string{{"display_name": "中", "remote_value": "medium"}}}
	f = p.call(t, "model.create", params)
	assertOK(t, f)
	var modelID idResult
	if err := json.Unmarshal(f.Result, &modelID); err != nil {
		t.Fatal(err)
	}
	params["id"] = modelID.ID
	params["enabled"] = false
	params["capabilities"] = nil
	params["model_type"] = []string{}
	params["reasoning_levels"] = nil
	assertOK(t, p.call(t, "model.update", params))
	f = p.call(t, "model.get", map[string]any{"id": modelID.ID})
	assertOK(t, f)
	if !bytes.Contains(f.Result, []byte(`"enabled":false`)) || !bytes.Contains(f.Result, []byte(`"capabilities":null`)) {
		t.Fatalf("零值更新失败: %s", f.Result)
	}
	f = p.call(t, "model.list", map[string]any{"selectable_only": true})
	assertOK(t, f)
	if string(f.Result) != "[]" {
		t.Fatal(string(f.Result))
	}
	assertOK(t, p.call(t, "model.set_enabled", map[string]any{"id": modelID.ID, "enabled": true}))
	runtime, err := store.Resolve(t.Context(), modelID.ID)
	if err != nil || runtime.APIKey != "fake-private-key" {
		t.Fatalf("省略密钥应保留: %v", err)
	}
	f = p.call(t, "provider.delete", map[string]any{"id": providerID.ID})
	if f.Error == nil || f.Error.Code != 1003 {
		t.Fatalf("关联删除未拒绝: %+v", f)
	}
	providerParams["api_key"] = ""
	assertOK(t, p.call(t, "provider.update", providerParams))
	f = p.call(t, "provider.get", map[string]any{"id": providerID.ID})
	assertOK(t, f)
	if !bytes.Contains(f.Result, []byte(`"has_api_key":false`)) {
		t.Fatal(string(f.Result))
	}
	assertOK(t, p.call(t, "model.delete", map[string]any{"id": modelID.ID}))
	assertOK(t, p.call(t, "provider.delete", map[string]any{"id": providerID.ID}))
}

func TestInvalidProtocolAndParameters(t *testing.T) {
	p := openPeer(t, serverForTest(t, storeForTest(t)))
	for _, test := range []struct {
		line string
		code int
	}{
		{`{"jsonrpc":`, -32700},
		{`null`, -32600},
		{string([]byte{'"', 0xff, '"'}), -32700},
		{`[]`, -32600},
		{`{"JSONRPC":"2.0","id":1,"method":"system.info"}`, -32600},
		{`{"jsonrpc":"2.0","id":true,"method":"system.info"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"missing"}`, -32601},
		{`{"jsonrpc":"2.0","id":1,"method":"system.info","params":{"unknown":true}}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"provider.create","params":{"name":"p","protocol":"chatcompletions","base_url":"https://example.invalid","api_key":null}}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"model.create","params":{"provider_id":"01900000-0000-7000-8000-000000000001","display_name":"m","remote_model_id":"m","enabled":true,"capabilities":{"tools":null}}}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"model.invoke","params":{"model_id":"01900000-0000-7000-8000-000000000001","messages":[{"role":"tool","content":"x"}]}}`, -32602},
	} {
		p.raw(t, test.line)
		f := p.next(t)
		if f.Error == nil || f.Error.Code != test.code {
			t.Fatalf("%s: %+v", test.line, f)
		}
	}
	// 合法通知不产生响应，下一条应直接是随后请求的结果。
	p.raw(t, `{"jsonrpc":"2.0","method":"missing"}`)
	assertOK(t, p.call(t, "system.info", map[string]any{}))
}

func TestResponsePrecedesStreamingAndGenerateEvents(t *testing.T) {
	p := openPeer(t, serverForTest(t, storeForTest(t)))
	for _, stream := range []bool{true, false} {
		f := p.call(t, "model.invoke", map[string]any{"model_id": "01900000-0000-7000-8000-000000000001", "stream": stream, "messages": []map[string]string{{"role": "user", "content": "你好"}}})
		assertOK(t, f)
		var accepted struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(f.Result, &accepted); err != nil || accepted.RunID == "" {
			t.Fatal("缺少执行标识")
		}
		f = p.next(t)
		if stream {
			if f.Method != "run.text_delta" {
				t.Fatalf("缺少增量: %+v", f)
			}
			f = p.next(t)
		}
		if f.Method != "run.completed" || !bytes.Contains(f.Params, []byte(accepted.RunID)) {
			t.Fatalf("终态错误: %+v", f)
		}
	}
}

func TestBlockedOutputAndInputCancellation(t *testing.T) {
	t.Run("输出阻塞", func(t *testing.T) {
		server := serverForTest(t, storeForTest(t))
		server.writeTimeout = 30 * time.Millisecond
		outR, outW := io.Pipe()
		defer outR.Close()
		err := server.Serve(t.Context(), io.NopCloser(strings.NewReader("")), outW)
		if !errors.Is(err, errSlowConsumer) {
			t.Fatalf("未报告阻塞: %v", err)
		}
	})
	t.Run("输入阻塞时取消", func(t *testing.T) {
		server := serverForTest(t, storeForTest(t))
		p := openPeer(t, server)
		p.cancel()
	})
	t.Run("超长输入", func(t *testing.T) {
		server := serverForTest(t, storeForTest(t))
		err := server.Serve(t.Context(), io.NopCloser(strings.NewReader(strings.Repeat("x", maxRequestBytes+10))), discardCloser{})
		if err == nil {
			t.Fatal("超长请求未拒绝")
		}
	})
}

type discardCloser struct{}

func (discardCloser) Write(p []byte) (int, error) { return len(p), nil }
func (discardCloser) Close() error                { return nil }

type failingClose struct {
	io.Reader
	err error
}

func (c failingClose) Close() error { return c.err }

type failingWrite struct{ err error }

func (c failingWrite) Write([]byte) (int, error) { return 0, c.err }
func (c failingWrite) Close() error              { return nil }
func TestCleanupAndBrokenOutputErrorsPreserved(t *testing.T) {
	server := serverForTest(t, storeForTest(t))
	closeErr := errors.New("关闭失败")
	err := server.Serve(t.Context(), failingClose{Reader: strings.NewReader(""), err: closeErr}, discardCloser{})
	if !errors.Is(err, closeErr) {
		t.Fatalf("关闭错误丢失: %v", err)
	}
	writeErr := errors.New("写入失败")
	err = server.Serve(t.Context(), io.NopCloser(strings.NewReader("")), failingWrite{writeErr})
	if !errors.Is(err, writeErr) {
		t.Fatalf("写入错误丢失: %v", err)
	}
}

func TestConnectionCloseIsRepeatableAndPreservesCleanupError(t *testing.T) {
	closeErr := errors.New("关闭输入失败")
	connection, err := NewConnection(t.Context(), failingClose{Reader: strings.NewReader(""), err: closeErr}, discardCloser{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := connection.Close(); !errors.Is(err, closeErr) {
			t.Fatalf("重复关闭丢失清理错误: %v", err)
		}
	}
}

type waitingClient struct{ started chan struct{} }

func (c waitingClient) Generate(ctx context.Context, _ model.Request) (model.Response, error) {
	close(c.started)
	<-ctx.Done()
	return model.Response{}, ctx.Err()
}
func (c waitingClient) Stream(ctx context.Context, _ model.Request, _ func(model.Event) error) error {
	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestEOFAndShutdownCancelAndDrainActiveExecution(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "EOF", true: "关闭请求"}[shutdown], func(t *testing.T) {
			server := serverForTest(t, storeForTest(t))
			started := make(chan struct{})
			server.resolve = func(context.Context, string) (model.Client, string, error) {
				return waitingClient{started}, "remote", nil
			}
			p := openPeer(t, server)
			assertOK(t, p.call(t, "model.invoke", map[string]any{"model_id": "01900000-0000-7000-8000-000000000001", "messages": []map[string]string{{"role": "user", "content": "等待"}}}))
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("执行未启动")
			}
			if shutdown {
				assertOK(t, p.call(t, "system.shutdown", map[string]any{}))
			} else {
				if err := p.input.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if f := p.next(t); f.Method != "run.cancelled" {
				t.Fatalf("未排空取消事件: %+v", f)
			}
		})
	}
}
