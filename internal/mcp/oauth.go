package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/xiaoyu98628/callum/internal/settings"
)

type OAuthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

type authorization struct {
	cancel context.CancelFunc
	view   OAuthStatus // 由 Service.mu 保护。
}

type oauthCallback struct {
	code string
	err  error
}

func (s *Service) StartOAuth(ctx, owner context.Context, id string) (OAuthStatus, error) {
	e, err := s.get(id)
	if err != nil {
		return OAuthStatus{}, err
	}
	if e.cfg.Auth != settings.MCPOAuth {
		return OAuthStatus{}, settings.ErrInvalid
	}
	ctx, cancelDiscovery := entryContext(ctx, e)
	defer cancelDiscovery()
	meta, resource, scope, err := s.discoverOAuth(ctx, e.cfg.URL)
	if err != nil {
		return OAuthStatus{}, err
	}
	uri, listener, err := listenCallback(ctx, e.cfg.RedirectURI)
	if err != nil {
		return OAuthStatus{}, ErrCallback
	}
	owned := true
	defer func() {
		if owned {
			_ = listener.Close()
		}
	}()
	credential := Credential{ClientID: e.cfg.ClientID, TokenURL: meta.TokenEndpoint, Resource: resource}
	if credential.ClientID == "" {
		if meta.RegistrationEndpoint == "" {
			return OAuthStatus{}, ErrClientRegistration
		}
		if _, err := settings.MCPHTTPS(meta.RegistrationEndpoint); err != nil {
			return OAuthStatus{}, ErrOAuth
		}
		grantTypes := []string{"authorization_code"}
		if slices.Contains(meta.GrantTypesSupported, "refresh_token") {
			grantTypes = append(grantTypes, "refresh_token")
		}
		reg, err := oauthex.RegisterClient(ctx, meta.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
			ClientName: "Callum", ApplicationType: "native", RedirectURIs: []string{uri.String()},
			TokenEndpointAuthMethod: "none", GrantTypes: grantTypes, ResponseTypes: []string{"code"},
		}, s.oauthHTTP)
		if err != nil {
			if e, ok := errors.AsType[*oauthex.ClientRegistrationError](err); ok && e.ErrorCode == "invalid_redirect_uri" {
				return OAuthStatus{}, ErrCallback
			}
			return OAuthStatus{}, ErrOAuth
		}
		if reg.ClientID == "" || !slices.Contains(reg.RedirectURIs, uri.String()) || reg.ClientSecret != "" {
			return OAuthStatus{}, ErrOAuth
		}
		credential.ClientID = reg.ClientID
	}
	verifier := base64.RawURLEncoding.EncodeToString(randBytes())
	state := base64.RawURLEncoding.EncodeToString(randBytes())
	challenge := sha256.Sum256([]byte(verifier))
	authURL, _ := settings.MCPHTTPS(meta.AuthorizationEndpoint)
	query := authURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", credential.ClientID)
	query.Set("redirect_uri", uri.String())
	query.Set("resource", resource)
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	if scope != "" {
		query.Set("scope", scope)
	}
	authURL.RawQuery = query.Encode()
	if len(authURL.String()) > 8192 {
		return OAuthStatus{}, ErrOAuth
	}
	flowCtx, cancel := context.WithTimeout(e.ctx, 5*time.Minute)
	stopOwner := context.AfterFunc(owner, cancel)
	flow := &authorization{cancel: cancel, view: OAuthStatus{Status: "pending"}}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.entries[id] != e || e.ctx.Err() != nil || owner.Err() != nil {
		stopOwner()
		cancel()
		return OAuthStatus{}, ErrChanged
	}
	if e.flow != nil {
		e.flow.cancel()
	}
	e.flow = flow
	s.wg.Go(func() {
		defer stopOwner()
		defer cancel()
		s.finishOAuth(flowCtx, e, flow, listener, uri, meta, state, verifier, credential)
	})
	owned = false
	return OAuthStatus{Status: "pending", URL: authURL.String()}, nil
}

func randBytes() []byte {
	out := make([]byte, 32)
	rand.Read(out)
	return out
}

func listenCallback(ctx context.Context, configured string) (*url.URL, net.Listener, error) {
	uri := &url.URL{Scheme: "http", Host: "127.0.0.1:0", Path: "/oauth/callback"}
	if configured != "" {
		uri, _ = url.Parse(configured)
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", uri.Host)
	if err != nil {
		return nil, nil, err
	}
	uri.Host = listener.Addr().String()
	return uri, listener, nil
}

func (s *Service) finishOAuth(ctx context.Context, e *entry, flow *authorization, listener net.Listener, uri *url.URL, meta *oauthex.AuthServerMeta, state, verifier string, credential Credential) {
	result := make(chan oauthCallback, 1)
	accepted := make(chan struct{}, 1)
	server := &http.Server{
		ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8192,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if r.Method != http.MethodGet || r.Host != uri.Host || r.URL.EscapedPath() != uri.EscapedPath() {
				http.Error(w, "无效的回调地址", http.StatusNotFound)
				return
			}
			q, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 ||
				len(q["iss"]) > 1 || ((q.Has("iss") || meta.AuthorizationResponseIssParameterSupported) && q.Get("iss") != meta.Issuer) {
				http.Error(w, "授权回调校验失败", http.StatusBadRequest)
				return
			}
			callback := oauthCallback{code: q.Get("code")}
			switch {
			case q.Get("error") == "invalid_redirect_uri" || q.Get("error") == "redirect_uri_mismatch":
				callback.err = ErrCallback
			case q.Get("error") == "access_denied":
				callback.err = errors.New("用户拒绝了 MCP 授权")
			case q.Has("error"), len(q["code"]) != 1, callback.code == "", len(callback.code) > 4096:
				callback.err = ErrOAuth
			}
			select {
			case accepted <- struct{}{}:
				_, _ = io.WriteString(w, "已收到授权响应。请返回 Callum 查看结果。")
				result <- callback
			default:
				http.Error(w, "该授权响应已经接收", http.StatusConflict)
			}
		}),
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	var cb oauthCallback
	served := false
	select {
	case <-ctx.Done():
		cb.err = ctx.Err()
	case cb = <-result:
	case <-finished:
		served = true
		cb.err = ErrCallback
	}
	if !served {
		// 收到授权码即关闭监听，Token 请求走独立的 HTTPS 连接。
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if server.Shutdown(closeCtx) != nil {
			_ = server.Close()
		}
		cancel()
		<-finished
	}
	if cb.err == nil {
		tokenCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		cb.err = s.completeOAuth(tokenCtx, e, flow, credential, uri.String(), cb.code, verifier)
		cancel()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.flow != flow {
		return
	}
	switch {
	case cb.err == nil:
		flow.view = OAuthStatus{Status: "authorized"}
	case errors.Is(cb.err, context.Canceled):
		flow.view = OAuthStatus{Status: "cancelled"}
	case errors.Is(cb.err, context.DeadlineExceeded):
		flow.view = OAuthStatus{Status: "failed", Message: "等待授权超时，请检查服务商是否接受本机回调"}
	default:
		flow.view = OAuthStatus{Status: "failed", Message: cb.err.Error()}
	}
}

func (s *Service) completeOAuth(ctx context.Context, e *entry, flow *authorization, c Credential, redirect, code, verifier string) error {
	next, err := s.tokenRequest(ctx, c, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	select {
	case e.tokenGate <- struct{}{}:
		defer func() { <-e.tokenGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.closed || s.entries[e.cfg.ID] != e || e.flow != flow {
		return context.Canceled
	}
	if err := s.repo.SaveCredential(ctx, e.cfg.ID, next); err != nil {
		return errors.New("无法安全保存 MCP 授权凭据")
	}
	e.credentialVersion.Add(1)
	return nil
}

func (s *Service) OAuthState(id string, cancel bool) (OAuthStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[id]
	if e == nil {
		return OAuthStatus{}, ErrNotFound
	}
	if e.flow == nil {
		return OAuthStatus{Status: "idle"}, nil
	}
	if cancel && e.flow.view.Status == "pending" {
		e.flow.view = OAuthStatus{Status: "cancelled"}
		e.flow.cancel()
	}
	return e.flow.view, nil
}

func (s *Service) discoverOAuth(ctx context.Context, endpoint string) (*oauthex.AuthServerMeta, string, string, error) {
	resource, _ := settings.MCPHTTPS(endpoint)
	var candidates []string
	scope := ""
	// 从实际 MCP 端点读取 OAuth challenge，其次尝试规范的 well-known 地址。
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"Callum","version":"1.0"}}}`)
	if req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body); err == nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if resp, err := s.http.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				challenges, _ := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
				for _, c := range challenges {
					if c.Scheme == "bearer" {
						scope = c.Params["scope"]
						if hint := c.Params["resource_metadata"]; hint != "" {
							candidates = append(candidates, hint)
						}
						break
					}
				}
			}
		}
	}
	candidates = append(candidates, wellKnown(resource, "/.well-known/oauth-protected-resource", true), wellKnown(resource, "/.well-known/oauth-protected-resource", false))
	for _, candidate := range candidates {
		if _, err := settings.MCPHTTPS(candidate); err != nil {
			continue
		}
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, candidate, endpoint, s.oauthHTTP)
		if err != nil {
			continue
		}
		for _, id := range prm.AuthorizationServers {
			issuer, err := settings.MCPHTTPS(id)
			if err != nil || issuer.RawQuery != "" {
				continue
			}
			for _, path := range []string{wellKnown(issuer, "/.well-known/oauth-authorization-server", true), wellKnown(issuer, "/.well-known/openid-configuration", true), issuer.String() + "/.well-known/openid-configuration"} {
				meta, err := oauthex.GetAuthServerMeta(ctx, path, id, s.oauthHTTP)
				if err != nil || !slices.Contains(meta.CodeChallengeMethodsSupported, "S256") {
					continue
				}
				if _, err := settings.MCPHTTPS(meta.AuthorizationEndpoint); err != nil {
					continue
				}
				if _, err := settings.MCPHTTPS(meta.TokenEndpoint); err != nil {
					continue
				}
				if scope == "" {
					scope = strings.Join(prm.ScopesSupported, " ")
				}
				return meta, prm.Resource, scope, nil
			}
		}
	}
	return nil, "", "", ErrOAuth
}

func wellKnown(origin *url.URL, prefix string, includePath bool) string {
	u := url.URL{Scheme: origin.Scheme, Host: origin.Host, Path: prefix}
	if includePath {
		u.Path += strings.TrimRight(origin.Path, "/")
	}
	return u.String()
}

func (s *Service) tokenRequest(ctx context.Context, c Credential, form url.Values) (Credential, error) {
	form.Set("client_id", c.ClientID)
	form.Set("resource", c.Resource)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, ErrOAuth
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.oauthHTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Credential{}, ctx.Err()
		}
		return Credential{}, ErrOAuth
	}
	defer resp.Body.Close()
	var result struct {
		Access  string `json:"access_token"`
		Type    string `json:"token_type"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
		Error   string `json:"error"`
	}
	dec := json.NewDecoder(limitBody(resp.Body))
	if dec.Decode(&result) != nil || dec.Decode(new(any)) != io.EOF {
		return Credential{}, ErrOAuth
	}
	switch result.Error {
	case "invalid_grant", "invalid_client":
		return Credential{}, ErrAuthorize
	case "invalid_redirect_uri", "redirect_uri_mismatch":
		return Credential{}, ErrCallback
	}
	if resp.StatusCode != http.StatusOK || result.Error != "" || !strings.EqualFold(result.Type, "Bearer") ||
		len(result.Access) > 8192 || len(result.Refresh) > 8192 || result.Expires < 0 || result.Expires > 1<<31-1 ||
		result.Access == "" || strings.ContainsAny(result.Access, "\x00\r\n \t") {
		return Credential{}, ErrOAuth
	}
	c.Token = result.Access
	c.Expires = time.Time{}
	if result.Refresh != "" {
		c.Refresh = result.Refresh
	}
	if result.Expires > 0 {
		c.Expires = time.Now().Add(time.Duration(result.Expires) * time.Second)
	}
	return c, nil
}

func (s *Service) accessToken(ctx context.Context, e *entry) (string, error) {
	select {
	case e.tokenGate <- struct{}{}:
		defer func() { <-e.tokenGate }()
	case <-ctx.Done():
		return "", ctx.Err()
	case <-e.ctx.Done():
		return "", ErrChanged
	}
	c, err := s.repo.Credential(ctx, e.cfg.ID)
	if err != nil {
		return "", err
	}
	if c.Token == "" {
		return "", ErrAuthorize
	}
	if c.Expires.IsZero() || time.Until(c.Expires) > 30*time.Second {
		return c.Token, nil
	}
	if c.Refresh == "" {
		if time.Until(c.Expires) > 0 {
			return c.Token, nil
		}
		return "", ErrAuthorize
	}
	tokenCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	next, err := s.tokenRequest(tokenCtx, c, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.Refresh}})
	if err != nil && !errors.Is(err, ErrAuthorize) {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.entries[e.cfg.ID] != e || e.ctx.Err() != nil {
		return "", ErrChanged
	}
	latest, loadErr := s.repo.Credential(ctx, e.cfg.ID)
	if loadErr != nil {
		return "", loadErr
	}
	if latest.Token != c.Token || latest.Refresh != c.Refresh {
		return latest.Token, nil
	}
	if writeErr := s.repo.SaveCredential(ctx, e.cfg.ID, next); writeErr != nil {
		return "", writeErr
	}
	return next.Token, err
}
