package settings

import (
	"encoding/json"
	"errors"
	"maps"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/xiaoyu98628/callum/internal/id"
)

const MCPKey = "mcp"

const (
	MCPStdio = "stdio"
	MCPHTTP  = "streamable_http"
	MCPNone  = "none"
	MCPToken = "token"
	MCPOAuth = "oauth"
)

var envName = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]{0,127}$")

// MCP 保存服务配置及工具允许清单。凭据由独立仓储保存。
type MCP struct {
	Servers []MCPServer `json:"servers"`
}

// MCPServer 是可以安全返回给设置界面的服务配置。
type MCPServer struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	Transport    string            `json:"transport"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Directory    string            `json:"directory,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	URL          string            `json:"url,omitempty"`
	Auth         string            `json:"auth"`
	ClientID     string            `json:"client_id,omitempty"`
	RedirectURI  string            `json:"redirect_uri,omitempty"`
	AllowedTools []string          `json:"allowed_tools"`

	// Scope 是远程 OAuth 服务显式请求的权限范围，空值表示自动发现。
	Scope string `json:"scope,omitempty"`
}

func DefaultMCP() MCP { return MCP{Servers: []MCPServer{}} }

// Clone 复制所有可变字段，使配置持有者与调用方分别拥有数据。
func (s MCPServer) Clone() MCPServer {
	s.Args = slices.Clone(s.Args)
	s.Env = maps.Clone(s.Env)
	s.AllowedTools = slices.Clone(s.AllowedTools)
	return s
}

// Clone 为整个配置创建独立快照，保留 nil 与空集合的区别。
func (v MCP) Clone() MCP {
	v.Servers = slices.Clone(v.Servers)
	for i := range v.Servers {
		v.Servers[i] = v.Servers[i].Clone()
	}
	return v
}

func DecodeMCP(raw json.RawMessage) (MCP, error) {
	var value MCP
	if len(raw) > MaxValueBytes || json.Unmarshal(raw, &value) != nil || value.Servers == nil || len(value.Servers) > 16 {
		return MCP{}, ErrInvalid
	}
	seen := make(map[string]bool, len(value.Servers))
	for _, server := range value.Servers {
		if !server.Valid() || seen[server.ID] {
			return MCP{}, ErrInvalid
		}
		seen[server.ID] = true
	}
	return value, nil
}

func DecodeMCPRecord(record Record) (MCP, error) {
	value, err := DecodeMCP(record.Value)
	if err != nil || record.Key != MCPKey || record.Revision < 1 {
		return MCP{}, errors.Join(ErrStorage, ErrInvalid)
	}
	return value, nil
}

func (s MCPServer) Valid() bool {
	if !id.ValidV7(s.ID) || strings.TrimSpace(s.Name) != s.Name || s.Name == "" || len(s.Name) > 240 || len(s.AllowedTools) > 128 {
		return false
	}
	if s.Scope != "" {
		if s.Transport != MCPHTTP || s.Auth != MCPOAuth {
			return false
		}
		if _, err := NormalizeMCPOAuthScope(s.Scope); err != nil {
			return false
		}
	}
	tools := map[string]bool{}
	for _, name := range s.AllowedTools {
		if name == "" || len(name) > 256 || strings.ContainsAny(name, "\x00\r\n") || tools[name] {
			return false
		}
		tools[name] = true
	}
	switch s.Transport {
	case MCPStdio:
		if s.Auth != MCPNone || s.URL != "" || s.ClientID != "" || s.RedirectURI != "" || !filepath.IsAbs(s.Command) || strings.ContainsRune(s.Command, 0) || len(s.Args) > 64 || len(s.Env) > 64 || (s.Directory != "" && (!filepath.IsAbs(s.Directory) || strings.ContainsRune(s.Directory, 0))) {
			return false
		}
		for _, arg := range s.Args {
			if strings.ContainsRune(arg, 0) {
				return false
			}
		}
		for key, val := range s.Env {
			if !ValidMCPEnv(key, val) {
				return false
			}
		}
		return true
	case MCPHTTP:
		if u, err := MCPHTTPS(s.URL); err != nil || u.RawQuery != "" || u.ForceQuery || s.Command != "" || len(s.Args) > 0 || s.Directory != "" || len(s.Env) > 0 {
			return false
		}
		if s.Auth != MCPOAuth {
			return (s.Auth == MCPNone || s.Auth == MCPToken) && s.ClientID == "" && s.RedirectURI == ""
		}
		if len(s.ClientID) > 1024 || strings.TrimSpace(s.ClientID) != s.ClientID || strings.ContainsAny(s.ClientID, "\x00\r\n") {
			return false
		}
		if s.RedirectURI == "" {
			return true
		}
		u, err := url.Parse(s.RedirectURI)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return false
		}
		port, err := strconv.ParseUint(u.Port(), 10, 16)
		return err == nil && port > 0
	}
	return false
}

// NormalizeMCPOAuthScope 校验权限名，并规范化空格、顺序和重复项。
func NormalizeMCPOAuthScope(raw string) (string, error) {
	if len(raw) > 1024 {
		return "", ErrInvalid
	}
	for _, ch := range raw {
		if ch < ' ' || ch > '~' || ch == '"' || ch == '\\' {
			return "", ErrInvalid
		}
	}
	values := strings.Fields(raw)
	if len(values) == 0 {
		return "", ErrInvalid
	}
	slices.Sort(values)
	return strings.Join(slices.Compact(values), " "), nil
}

func ValidMCPEnv(key, value string) bool {
	return envName.MatchString(key) && len(value) <= 4096 && !strings.ContainsRune(value, 0)
}

// MCPHTTPS 限定可配置的 HTTPS 端点，禁止凭据和片段混入地址。
func MCPHTTPS(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrInvalid
	}
	return u, nil
}

func (v MCP) Server(id string) (MCPServer, bool) {
	i := slices.IndexFunc(v.Servers, func(server MCPServer) bool { return server.ID == id })
	if i < 0 {
		return MCPServer{}, false
	}
	return v.Servers[i], true
}
