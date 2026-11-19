package mcp_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/credentials"
	"github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
	"github.com/xiaoyu98628/callum/internal/mcp"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	mcpstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/mcp"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestStreamableHTTPOAuthUsesLoopbackAndEncryptedStorage(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		name := "random"
		if fixed {
			name = "fixed"
		}
		t.Run(name, func(t *testing.T) {
			testStreamableOAuth(t, fixed)
		})
	}
}

func testStreamableOAuth(t *testing.T, fixed bool) {
	ctx := t.Context()
	const access = "controlled-access-token"
	const refresh = "controlled-refresh-token"
	const rotated = "controlled-rotated-access-token"
	var tokens, toolCalls, registrations atomic.Int32
	var authChallenge atomic.Value
	tokenRequested := make(chan struct{}, 1)
	releaseToken := make(chan struct{})

	sdkServer := sdk.NewServer(&sdk.Implementation{Name: "回显服务", Version: "1"}, nil)
	sdkServer.AddTool(&sdk.Tool{Name: "echo", Description: "受控工具", InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}, func(_ context.Context, r *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		toolCalls.Add(1)
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "mcp-http-ok"}}}, nil
	})
	mcpHandler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return sdkServer }, &sdk.StreamableHTTPOptions{JSONResponse: fixed})
	mux := http.NewServeMux()
	var issuer string
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+access && r.Header.Get("Authorization") != "Bearer "+rotated {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+issuer+`/.well-known/oauth-protected-resource/mcp", scope="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// 模拟 HTTPS 名称通过受控拨号落到临时回环地址；服务端匹配实际本机 Host。
		r.Host = r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()
		mcpHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"resource": issuer + "/mcp", "authorization_servers": []string{issuer}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "registration_endpoint": issuer + "/register", "code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		registrations.Add(1)
		var in struct {
			RedirectURIs []string `json:"redirect_uris"`
			Auth         string   `json:"token_endpoint_auth_method"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&in) != nil || len(in.RedirectURIs) != 1 || in.Auth != "none" {
			t.Error("OAuth 动态注册参数错误")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "test-client", "redirect_uris": in.RedirectURIs, "token_endpoint_auth_method": "none"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokens.Add(1)
		if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("client_id") != "test-client" || r.Form.Get("resource") != issuer+"/mcp" {
			t.Error("令牌参数无效")
			w.WriteHeader(400)
			return
		}
		if r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") == refresh {
			writeJSON(t, w, map[string]any{"access_token": rotated, "token_type": "Bearer", "expires_in": 3600})
			return
		}
		if r.Form.Get("code") == "controlled-failed-code" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		expected := authChallenge.Load().(url.Values)
		hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "controlled-code" ||
			r.Form.Get("redirect_uri") != expected.Get("redirect_uri") ||
			base64.RawURLEncoding.EncodeToString(hash[:]) != expected.Get("code_challenge") {
			t.Error("授权码、回调地址或 PKCE 与发起请求不符")
		}
		tokenRequested <- struct{}{}
		select {
		case <-r.Context().Done():
			return
		case <-releaseToken:
		}
		writeJSON(t, w, map[string]any{"access_token": access, "token_type": "Bearer", "refresh_token": refresh, "expires_in": 3600})
	})
	server := httptest.NewUnstartedServer(mux)
	server.StartTLS()
	t.Cleanup(server.Close)
	issuer = strings.Replace(server.URL, "127.0.0.1", "example.com", 1)
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	path := filepath.Join(t.TempDir(), "data", "callum.db")
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	vault, err := credentials.New(credentialstest.Keyring{}, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vault.Close() })
	repo := mcpstore.New(db.DB, vault)
	service, err := mcp.New(ctx, ctx, repo, client, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	cfg := settings.MCPServer{Name: "OAuth 服务", Transport: settings.MCPHTTP, Auth: settings.MCPOAuth, URL: issuer + "/mcp",
		Enabled: true, AllowedTools: []string{"echo"}}
	if fixed {
		cfg.ClientID = "test-client"
	}
	if fixed {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		cfg.RedirectURI = "http://" + listener.Addr().String() + "/return"
		id, err := service.Save(ctx, 1, cfg, mcp.SecretInput{}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.StartOAuth(ctx, ctx, id); err != mcp.ErrCallback {
			t.Fatalf("占用的固定端口: %v", err)
		}
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		cfg.ID = id
	}
	rev := int64(1)
	if fixed {
		rev++
	}
	id, err := service.Save(ctx, rev, cfg, mcp.SecretInput{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if fixed {
		if _, err := service.StartOAuth(ctx, ctx, id); err != nil {
			t.Fatal(err)
		}
		status, err := service.OAuthState(id, true)
		if err != nil || status.Status != "cancelled" {
			t.Fatalf("取消授权失败: %+v %v", status, err)
		}
		address, _ := url.Parse(cfg.RedirectURI)
		for deadline := time.Now().Add(2 * time.Second); ; {
			listener, err := net.Listen("tcp4", address.Host)
			if err == nil {
				_ = listener.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("取消后仍占用回调端口")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	state, err := service.StartOAuth(ctx, ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if (registrations.Load() == 1) == fixed {
		t.Fatal("OAuth 没有按服务配置使用预注册或动态登记")
	}
	authorization, _ := url.Parse(state.URL)
	q := authorization.Query()
	callback, _ := url.Parse(q.Get("redirect_uri"))
	if callback.Hostname() != "127.0.0.1" || callback.Port() == "" || callback.Port() == "0" ||
		(fixed && callback.String() != cfg.RedirectURI) || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("授权地址配置错误: %s", state.URL)
	}
	authChallenge.Store(q)
	browser := http.Client{Timeout: 2 * time.Second}
	callback.RawQuery = url.Values{"state": {"wrong-state"}, "code": {"controlled-code"}, "iss": {issuer}}.Encode()
	invalid, err := browser.Get(callback.String())
	if err != nil {
		t.Fatal(err)
	}
	checkOAuthPage(t, invalid, http.StatusBadRequest, "failed", "授权回调校验失败", "wrong-state", "controlled-code")
	if tokens.Load() != 0 {
		t.Fatal("无效 state 触发了换令牌")
	}

	callback.RawQuery = url.Values{"state": {q.Get("state")}, "code": {"controlled-code"}, "iss": {issuer}}.Encode()
	type browserResult struct {
		response *http.Response
		err      error
	}
	reply := make(chan browserResult, 1)
	go func() {
		response, err := browser.Get(callback.String())
		reply <- browserResult{response, err}
	}()
	select {
	case <-tokenRequested:
	case <-time.After(2 * time.Second):
		close(releaseToken)
		t.Fatal("浏览器回调未开始换取令牌")
	}
	pending, err := service.OAuthState(id, false)
	if err != nil || pending.Status != "pending" {
		close(releaseToken)
		t.Fatalf("换令牌期间状态不正确: %+v %v", pending, err)
	}
	// 模拟授权端仍在处理时，浏览器不能收到声称登录成功的页面。
	select {
	case early := <-reply:
		close(releaseToken)
		if early.response != nil {
			_ = early.response.Body.Close()
		}
		t.Fatalf("换令牌完成前浏览器已经收到响应: %v", early.err)
	default:
		close(releaseToken)
	}
	browserReply := <-reply
	if browserReply.err != nil {
		t.Fatal(browserReply.err)
	}
	checkOAuthPage(t, browserReply.response, http.StatusOK, "authorized", "授权成功", q.Get("state"), "controlled-code", access)
	status, err := service.OAuthState(id, false)
	if err != nil || status.Status != "authorized" {
		t.Fatalf("浏览器和桌面端授权状态不一致: %+v %v", status, err)
	}

	stored, err := repo.Credential(ctx, id)
	if err != nil || stored.Token != access || stored.Refresh != refresh {
		t.Fatalf("凭据未正确保存: %v", err)
	}
	var row schema.MCPCredential
	if err := db.DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row.Ciphertext, access) || strings.Contains(row.Ciphertext, refresh) || !strings.HasPrefix(row.Ciphertext, "v1:") {
		t.Fatal("凭据未以密文保存")
	}
	catalog, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(catalog)
	if strings.Contains(string(public), access) || !catalog.Servers[0].Authorized {
		t.Fatal("公开目录泄漏凭据或授权状态有误")
	}
	stored.Expires = time.Now().Add(-time.Second)
	if err := repo.SaveCredential(ctx, id, stored); err != nil {
		t.Fatal(err)
	}
	list, err := service.Test(ctx, id, false, rev+1)
	if err != nil || len(list) != 1 || list[0].Name != "echo" {
		t.Fatalf("发现失败: %+v %v", list, err)
	}
	tools := service.ToolsSnapshot(ctx, true, rev+1)
	if len(tools) != 1 {
		t.Fatal("允许的工具未进入运行快照")
	}
	valid, err := tools[0].Validate(json.RawMessage(`{"value":"sample"}`))
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := tools[0].Prepare(ctx, tool.Scope{}, valid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = invocation.Close() })
	result, err := invocation.Execute(ctx)
	if err != nil || toolCalls.Load() != 1 || !strings.Contains(string(result.Output), "mcp-http-ok") || tokens.Load() != 2 {
		t.Fatalf("HTTP 工具或刷新请求未完成: %+v %v", result, err)
	}
	if !fixed {
		for _, step := range []struct {
			params  url.Values
			status  string
			message string
		}{
			{url.Values{"error": {"access_denied"}, "error_description": {"remote-secret-error"}}, "cancelled", "已取消授权"},
			{url.Values{"code": {"controlled-failed-code"}}, "failed", "授权失败"},
		} {
			login, err := service.StartOAuth(ctx, ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			authURL, _ := url.Parse(login.URL)
			redirect, _ := url.Parse(authURL.Query().Get("redirect_uri"))
			callbackState := authURL.Query().Get("state")
			step.params.Set("state", callbackState)
			step.params.Set("iss", issuer)
			redirect.RawQuery = step.params.Encode()
			res, err := browser.Get(redirect.String())
			if err != nil {
				t.Fatal(err)
			}
			checkOAuthPage(t, res, http.StatusOK, step.status, step.message, callbackState, "remote-secret-error", "controlled-failed-code")
			status, err := service.OAuthState(id, false)
			if err != nil || status.Status != step.status {
				t.Fatalf("浏览器和桌面端授权状态不一致: %+v %v", status, err)
			}
		}
		if tokens.Load() != 3 {
			t.Fatalf("授权拒绝不应访问换令牌接口，实际令牌请求数: %d", tokens.Load())
		}
	}
	if err := service.Delete(ctx, rev+1, id); err != nil {
		t.Fatal(err)
	}
	removed, err := repo.Credential(ctx, id)
	if err != nil || !removed.Empty() {
		t.Fatal("删除服务没有同步清除凭据")
	}
	if fixed {
		cfg.ID, cfg.Auth, cfg.RedirectURI, cfg.ClientID = "", settings.MCPToken, "", ""
		manualID, err := service.Save(ctx, rev+2, cfg, mcp.SecretInput{Token: new(access)}, false)
		if err != nil {
			t.Fatal(err)
		}
		if tools, err := service.Test(ctx, manualID, false, rev+3); err != nil || len(tools) != 1 {
			t.Fatalf("手填访问令牌无法连接: %v", err)
		}
		if err := service.Disconnect(ctx, manualID); err != nil {
			t.Fatal(err)
		}
		if credential, err := repo.Credential(ctx, manualID); err != nil || !credential.Empty() {
			t.Fatal("清除访问凭据未生效")
		}
	}

}

func checkOAuthPage(t *testing.T, response *http.Response, code int, status, heading string, sensitive ...string) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)
	if response.StatusCode != code || response.Header.Get("Content-Type") != "text/html; charset=utf-8" ||
		response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" ||
		!strings.Contains(page, "data-status=\""+status+"\"") || !strings.Contains(page, heading) {
		t.Fatalf("授权结果页与预期不符: %d %v", response.StatusCode, response.Header)
	}
	_, nonceAndPolicy, ok := strings.Cut(response.Header.Get("Content-Security-Policy"), "script-src 'nonce-")
	nonce, _, ended := strings.Cut(nonceAndPolicy, "'")
	if !ok || !ended || len(nonce) == 0 || !strings.Contains(page, "<script nonce=\""+nonce+"\">") ||
		!strings.Contains(page, "history.replaceState") {
		t.Fatal("缺少清理地址栏参数的受信脚本")
	}
	for _, secret := range sensitive {
		if secret != "" && strings.Contains(page, secret) {
			t.Fatal("授权结果页包含回调参数或凭据")
		}
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Error(err)
	}
}
