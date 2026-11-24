// Package mcp 管理第三方服务、连接、工具目录和本机 OAuth。
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/settings"
)

var (
	ErrNotFound           = errors.New("MCP 服务不存在")
	ErrAuthorize          = errors.New("请先为 MCP 服务授权或更新访问令牌")
	ErrConnection         = errors.New("无法连接 MCP 服务，请检查地址、权限或本地命令")
	ErrChanged            = errors.New("MCP 配置或工具定义已变化，请刷新后重试")
	ErrCallback           = errors.New("无法使用本机 OAuth 回调，请检查回调地址、端口及服务商的登记要求")
	ErrClientRegistration = errors.New("服务商需要预注册 OAuth 客户端，请填写客户端 ID")
	ErrOAuth              = errors.New("MCP 授权失败，请检查服务商的 OAuth 配置")
	ErrOAuthScope         = errors.New("服务商拒绝了 MCP OAuth 权限范围，请检查 scope 配置")
	ErrOAuthScopeMismatch = errors.New("服务商授予的 MCP OAuth 权限范围与 scope 配置不一致")
)

// Credential 只在内部传递，由仓储在写入前加密。
type Credential struct {
	Token    string            `json:"token,omitempty"`
	Refresh  string            `json:"refresh,omitempty"`
	Expires  time.Time         `json:"expires,omitzero"`
	ClientID string            `json:"client_id,omitempty"`
	TokenURL string            `json:"token_url,omitempty"`
	Resource string            `json:"resource,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}

func (c Credential) Empty() bool {
	return c.Token == "" && len(c.Env) == 0
}

type Repository interface {
	Load(context.Context) (settings.MCP, int64, error)
	Save(context.Context, settings.MCP, int64, string, *Credential) error
	Credential(context.Context, string) (Credential, error)
	SaveCredential(context.Context, string, Credential) error
}

type SecretInput struct {
	Token *string            `json:"token,omitempty"`
	Env   *map[string]string `json:"env,omitempty"`
}

// SaveOptions 区分旧客户端省略 scope 与明确恢复自动发现。
type SaveOptions struct {
	Confirmed  bool
	ClearScope bool
}

type View struct {
	settings.MCPServer
	Connected     bool       `json:"connected"`
	Authorized    bool       `json:"authorized"`
	ExpiresAt     int64      `json:"expires_at,omitempty"`
	SecretEnvKeys []string   `json:"secret_env_keys"`
	Tools         []ToolInfo `json:"tools"`
}
type Catalog struct {
	Revision    int64  `json:"revision"`
	Servers     []View `json:"servers"`
	ScopeUpdate bool   `json:"scope_update"`
}

type connection struct {
	catalog           atomic.Pointer[toolCatalog]
	credentialVersion uint64
	session           *sdk.ClientSession
	dirty             atomic.Bool
}

type entry struct {
	publicConnection  atomic.Pointer[connection]
	credentialVersion atomic.Uint64
	cfg               settings.MCPServer

	ctx        context.Context
	cancel     context.CancelFunc
	lock       chan struct{}
	tokenGate  chan struct{}
	connection *connection    // 仅持有 lock 时访问。
	flow       *authorization // 由 Service.mu 保护。
}

func (e *entry) acquire(ctx context.Context) error {
	select {
	case e.lock <- struct{}{}:
		if err := e.ctx.Err(); err != nil {
			e.release()
			return ErrChanged
		}
		return nil
	case <-e.ctx.Done():
		return ErrChanged
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (e *entry) release() { <-e.lock }

// Service 由 Go 运行时持有。连接代际在保存、禁用和删除后立即取消。
type Service struct {
	ctx       context.Context
	cancel    context.CancelFunc
	repo      Repository
	http      *http.Client
	oauthHTTP *http.Client
	logger    *slog.Logger
	mu        sync.Mutex
	wg        sync.WaitGroup
	cfg       settings.MCP
	revision  int64

	entries map[string]*entry
	closed  bool
}

// New 加载保存的配置并创建运行期服务，使用调用方注入的日志记录器。
func New(initCtx, ctx context.Context, repo Repository, client, oauthClient *http.Client, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		return nil, errors.New("MCP 服务日志未配置")
	}
	cfg, revision, err := repo.Load(initCtx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{ctx: ctx, cancel: cancel, repo: repo, http: client, oauthHTTP: oauthClient,
		logger: logger.With("module", "mcp"), cfg: cfg, revision: revision, entries: make(map[string]*entry)}
	for _, item := range cfg.Servers {
		s.add(item)
	}
	return s, nil
}

func (s *Service) add(cfg settings.MCPServer) {
	ctx, cancel := context.WithCancel(s.ctx)
	s.entries[cfg.ID] = &entry{cfg: cfg, ctx: ctx, cancel: cancel, lock: make(chan struct{}, 1), tokenGate: make(chan struct{}, 1)}
}

func (s *Service) get(id string) (*entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrConnection
	}
	e := s.entries[id]
	if e == nil {
		return nil, ErrNotFound
	}
	return e, nil
}

func (s *Service) retire(e *entry) {
	e.cancel()
	if e.flow != nil {
		e.flow.cancel()
	}
	s.wg.Go(func() {
		e.lock <- struct{}{}
		defer e.release()
		s.closeConnection(e)
	})
}

// Close 停止受理请求并等待本服务持有的会话和授权任务结束，可重复调用。
func (s *Service) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancel()
		for _, e := range s.entries {
			s.retire(e)
		}
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) List(ctx context.Context) (Catalog, error) {
	s.mu.Lock()
	rev := s.revision
	entries := make([]*entry, 0, len(s.entries))
	for _, cfg := range s.cfg.Servers {
		entries = append(entries, s.entries[cfg.ID])
	}
	s.mu.Unlock()
	out := Catalog{Revision: rev, Servers: make([]View, 0, len(entries)), ScopeUpdate: true}
	for _, e := range entries {
		secret, err := s.repo.Credential(ctx, e.cfg.ID)
		if err != nil {
			return Catalog{}, err
		}
		view := View{MCPServer: e.cfg, Authorized: secret.Token != "" && (secret.Expires.IsZero() || time.Now().Before(secret.Expires) || secret.Refresh != ""), SecretEnvKeys: []string{}, Tools: []ToolInfo{}}
		if !secret.Expires.IsZero() {
			view.ExpiresAt = secret.Expires.UnixMilli()
		}
		view.SecretEnvKeys = append(view.SecretEnvKeys, slices.Sorted(maps.Keys(secret.Env))...)
		if conn := e.publicConnection.Load(); conn != nil && e.ctx.Err() == nil && conn.credentialVersion == e.credentialVersion.Load() {
			view.Connected = true
			if catalog := conn.catalog.Load(); catalog != nil && !conn.dirty.Load() {
				view.Tools = slices.Clone(catalog.public)
			}
		}
		out.Servers = append(out.Servers, view)
	}
	return out, nil
}

func (s *Service) Save(ctx context.Context, revision int64, cfg settings.MCPServer, secrets SecretInput, options SaveOptions) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrConnection
	}
	if revision != s.revision {
		return "", settings.ErrConflict
	}
	old := s.entries[cfg.ID]
	if cfg.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		cfg.ID = id.String()
	} else if old == nil {
		return "", ErrNotFound
	}
	if cfg.AllowedTools == nil {
		cfg.AllowedTools = []string{}
	}
	if options.ClearScope && (cfg.Scope != "" || cfg.Transport != settings.MCPHTTP || cfg.Auth != settings.MCPOAuth) {
		return "", settings.ErrInvalid
	}
	if !options.ClearScope && cfg.Scope == "" && old != nil && old.cfg.Scope != "" {
		previous := old.cfg
		previous.Scope = ""
		if sameIdentity(previous, cfg) {
			cfg.Scope = old.cfg.Scope
		}
	}
	if !cfg.Valid() {
		return "", settings.ErrInvalid
	}
	if cfg.Scope != "" {
		cfg.Scope, _ = settings.NormalizeMCPOAuthScope(cfg.Scope) // Valid 已校验权限范围。
	}
	var credential *Credential
	changed := old != nil && !sameIdentity(old.cfg, cfg)
	if cfg.Transport == settings.MCPStdio && cfg.Enabled && (old == nil || !old.cfg.Enabled || changed || secrets.Env != nil) && !options.Confirmed {
		return "", settings.ErrInvalid
	}
	if changed {
		cfg.AllowedTools = []string{}
	}
	if changed || secrets.Token != nil || secrets.Env != nil {
		value := Credential{}
		if old != nil && !changed {
			var err error
			value, err = s.repo.Credential(ctx, cfg.ID)
			if err != nil {
				return "", err
			}
		}
		if secrets.Token != nil {
			if cfg.Auth != settings.MCPToken || len(*secrets.Token) > 8192 || strings.ContainsAny(*secrets.Token, "\x00\r\n \t") {
				return "", settings.ErrInvalid
			}
			value.Token = *secrets.Token
		}
		if secrets.Env != nil {
			if cfg.Transport != settings.MCPStdio || len(*secrets.Env) > 64 {
				return "", settings.ErrInvalid
			}
			for key, val := range *secrets.Env {
				_, duplicate := cfg.Env[key]
				if duplicate || !settings.ValidMCPEnv(key, val) {
					return "", settings.ErrInvalid
				}
			}
			value.Env = *secrets.Env
		}
		credential = &value
	}
	if old != nil && credential == nil && cfg.Transport == settings.MCPStdio {
		value, err := s.repo.Credential(ctx, cfg.ID)
		if err != nil {
			return "", err
		}
		for key := range cfg.Env {
			if _, ok := value.Env[key]; ok {
				return "", settings.ErrInvalid
			}
		}
	}
	next := settings.MCP{Servers: slices.Clone(s.cfg.Servers)}
	if old == nil {
		next.Servers = append(next.Servers, cfg)
	} else {
		i := slices.IndexFunc(next.Servers, func(v settings.MCPServer) bool { return v.ID == cfg.ID })
		next.Servers[i] = cfg
	}
	if err := s.repo.Save(ctx, next, revision, cfg.ID, credential); err != nil {
		return "", err
	}
	s.cfg, s.revision = next, revision+1
	if old != nil {
		s.retire(old)
	}
	s.add(cfg)
	return cfg.ID, nil
}

func sameIdentity(a, b settings.MCPServer) bool {
	a.ID, a.Name, a.Enabled, a.AllowedTools = "", "", false, nil
	b.ID, b.Name, b.Enabled, b.AllowedTools = "", "", false, nil
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}

func (s *Service) Delete(ctx context.Context, revision int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrConnection
	}
	if revision != s.revision {
		return settings.ErrConflict
	}
	e := s.entries[id]
	if e == nil {
		return ErrNotFound
	}
	next := settings.MCP{Servers: slices.DeleteFunc(slices.Clone(s.cfg.Servers), func(v settings.MCPServer) bool { return v.ID == id })}
	if err := s.repo.Save(ctx, next, revision, id, &Credential{}); err != nil {
		return err
	}
	s.cfg, s.revision = next, revision+1
	delete(s.entries, id)
	s.retire(e)
	return nil
}

func (s *Service) Disconnect(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrConnection
	}
	e := s.entries[id]
	if e == nil {
		return ErrNotFound
	}
	if e.cfg.Transport != settings.MCPHTTP || e.cfg.Auth == settings.MCPNone {
		return settings.ErrInvalid
	}
	if err := s.repo.SaveCredential(ctx, id, Credential{}); err != nil {
		return err
	}
	s.retire(e)
	s.add(e.cfg)
	return nil
}

func (s *Service) Test(ctx context.Context, id string, confirmed bool, revision int64) ([]ToolInfo, error) {
	s.mu.Lock()
	e, current := s.entries[id], s.revision
	s.mu.Unlock()
	if e == nil {
		return nil, ErrNotFound
	}
	if e.cfg.Transport == settings.MCPStdio {
		if !confirmed {
			return nil, settings.ErrInvalid
		}
		if revision != current {
			return nil, settings.ErrConflict
		}
	}
	if err := e.acquire(ctx); err != nil {
		return nil, err
	}
	defer e.release()
	ctx, cancel := entryContext(ctx, e)
	defer cancel()
	if e.connection != nil && e.cfg.Scope != "" {
		// 复用会话时提前刷新，以便把 scope 错误直接返回给设置页。
		if _, err := s.accessToken(ctx, e); err != nil {
			s.closeConnection(e)
			return nil, err
		}
	}
	c, err := s.connect(ctx, e)
	if err != nil {
		return nil, err
	}
	if err := s.discover(ctx, e, c); err != nil {
		s.closeConnection(e)
		return nil, err
	}
	out := slices.Clone(c.catalog.Load().public)
	if !e.cfg.Enabled {
		s.closeConnection(e)
	}
	return out, nil
}

func entryContext(ctx context.Context, e *entry) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(e.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

// closeConnection 在持有条目锁时撤销快照并关闭会话，只记录脱敏的清理诊断。
func (s *Service) closeConnection(e *entry) {
	e.publicConnection.Store(nil)
	c := e.connection
	e.connection = nil
	if c == nil {
		return
	}
	if err := c.session.Close(); unexpectedCloseError(err) {
		kind := "close_failed"
		if errors.Is(err, context.DeadlineExceeded) {
			kind = "close_timeout"
		}
		s.logger.Warn("关闭 MCP 会话失败", "server_id", e.cfg.ID, "kind", kind)
	}
}

func unexpectedCloseError(err error) bool {
	if err == nil {
		return false
	}
	// 本地进程等传输可能合并清理错误，预期取消不能遮蔽同批次的其他失败。
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return slices.ContainsFunc(joined.Unwrap(), unexpectedCloseError)
	}
	if cause := errors.Unwrap(err); cause != nil {
		return unexpectedCloseError(cause)
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, ErrChanged) &&
		!errors.Is(err, sdk.ErrConnectionClosed) && !errors.Is(err, sdk.ErrSessionMissing)
}

func (s *Service) connect(ctx context.Context, e *entry) (*connection, error) {
	if e.connection != nil {
		if e.connection.credentialVersion == e.credentialVersion.Load() {
			return e.connection, nil
		}
		s.closeConnection(e)
	}
	c := &connection{credentialVersion: e.credentialVersion.Load()}
	var transport sdk.Transport
	switch e.cfg.Transport {
	case settings.MCPStdio:
		creds, err := s.repo.Credential(ctx, e.cfg.ID)
		if err != nil {
			return nil, err
		}
		transport = &processTransport{cfg: e.cfg, secrets: creds.Env}
	case settings.MCPHTTP:
		client := *s.http
		base := client.Transport
		client.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
			if e.credentialVersion.Load() != c.credentialVersion {
				return nil, ErrChanged
			}
			if e.cfg.Auth != settings.MCPNone {
				token, err := s.accessToken(req.Context(), e)
				if err != nil {
					return nil, err
				}
				if e.credentialVersion.Load() != c.credentialVersion {
					return nil, ErrChanged
				}
				req = req.Clone(req.Context())
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := base.RoundTrip(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode == http.StatusUnauthorized {
				_ = resp.Body.Close()
				return nil, ErrAuthorize
			}
			if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
				resp.Body = limitBody(resp.Body)
			}
			return resp, nil
		})
		transport = &sdk.StreamableClientTransport{Endpoint: e.cfg.URL, HTTPClient: &client, MaxEventSize: 1 << 20}
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "Callum", Version: "1.0"}, &sdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) { c.dirty.Store(true) },
	})
	session, err := client.Connect(ctx, transport, &sdk.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		if errors.Is(err, ErrAuthorize) {
			return nil, ErrAuthorize
		}
		if errors.Is(err, ErrOAuthScope) {
			return nil, ErrOAuthScope
		}
		if errors.Is(err, ErrOAuthScopeMismatch) {
			return nil, ErrOAuthScopeMismatch
		}
		return nil, ErrConnection
	}
	c.session = session
	e.connection = c
	e.publicConnection.Store(c)
	s.mu.Lock()
	if s.closed || e.ctx.Err() != nil {
		s.mu.Unlock()
		s.closeConnection(e)
		return nil, ErrChanged
	}
	s.wg.Go(func() {
		// Wait 用于感知连接结束，显式 Close 读取 SDK 单独保留的关闭错误。
		_ = session.Wait()
		c.dirty.Store(true)
		e.publicConnection.CompareAndSwap(c, nil)
		select {
		case e.lock <- struct{}{}:
			if e.connection == c {
				s.closeConnection(e)
			}
			e.release()
		case <-e.ctx.Done():
		}
	})
	s.mu.Unlock()
	return c, nil
}

// processEnvironment 避免把 Go 服务继承的其他凭据传给第三方进程。
func processEnvironment(values, secrets map[string]string) []string {
	env := map[string]string{}
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "TMP", "TEMP", "TMPDIR", "SystemRoot", "WINDIR", "APPDATA", "LOCALAPPDATA", "LANG"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	maps.Copy(env, values)
	maps.Copy(env, secrets)
	out := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		out = append(out, key+"="+env[key])
	}
	return out
}
