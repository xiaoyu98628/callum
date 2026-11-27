```go
package 设置

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	映射包 "maps"
	URL包 "net/url"
	文件路径包 "path/filepath"
	正则表达式包 "regexp"
	切片包 "slices"
	字符串转换包 "strconv"
	字符串包 "strings"

	标识包 "github.com/xiaoyu98628/callum/internal/id"
)

const MCP键 = "mcp"

const (
	MCP标准输入输出 = "stdio"
	MCPHTTP   = "streamable_http"
	MCP无      = "none"
	MCP词元     = "token"
	MCPOAuth  = "oauth"
)

var 环境变量名称 = 正则表达式包.必须编译("^[A-Za-z_][A-Za-z0-9_]{0,127}$")

// MCP 保存服务配置及工具允许清单。凭据由独立仓储保存。
type MCP struct {
	服务集 []MCP服务端 `json:"servers"`
}

// MCP服务端 是可以安全返回给设置界面的服务配置。
type MCP服务端 struct {
	标识     string            `json:"id"`
	名称     string            `json:"name"`
	已启用    bool              `json:"enabled"`
	传输     string            `json:"transport"`
	命令     string            `json:"command,omitempty"`
	启动参数   []string          `json:"args,omitempty"`
	目录     string            `json:"directory,omitempty"`
	环境变量   map[string]string `json:"env,omitempty"`
	URL    string            `json:"url,omitempty"`
	认证     string            `json:"auth"`
	客户端标识  string            `json:"client_id,omitempty"`
	重定向URI string            `json:"redirect_uri,omitempty"`
	允许工具集  []string          `json:"allowed_tools"`

	// 范围 是远程 OAuth 服务显式请求的权限范围，空值表示自动发现。
	范围 string `json:"scope,omitempty"`
}

func 默认MCP() MCP { return MCP{服务集: []MCP服务端{}} }

// 复制 复制所有可变字段，使配置持有者与调用方分别拥有数据。
func (当前MCP服务端 MCP服务端) 复制() MCP服务端 {
	当前MCP服务端.启动参数 = 切片包.复制(当前MCP服务端.启动参数)
	当前MCP服务端.环境变量 = 映射包.复制(当前MCP服务端.环境变量)
	当前MCP服务端.允许工具集 = 切片包.复制(当前MCP服务端.允许工具集)
	return 当前MCP服务端
}

// 复制 为整个配置创建独立快照，保留 nil 与空集合的区别。
func (当前MCP MCP) 复制() MCP {
	当前MCP.服务集 = 切片包.复制(当前MCP.服务集)
	for 索引 := range 当前MCP.服务集 {
		当前MCP.服务集[索引] = 当前MCP.服务集[索引].复制()
	}
	return 当前MCP
}

func 解码MCP(原始 JSON包.原始JSON消息) (MCP, error) {
	var 值 MCP
	if len(原始) > 最大值字节 || JSON包.反序列化(原始, &值) != nil || 值.服务集 == nil || len(值.服务集) > 16 {
		return MCP{}, 无效错误
	}
	已见 := make(map[string]bool, len(值.服务集))
	for _, 服务端 := range 值.服务集 {
		if !服务端.有效() || 已见[服务端.标识] {
			return MCP{}, 无效错误
		}
		已见[服务端.标识] = true
	}
	return 值, nil
}

func 解码MCP记录(记录 记录) (MCP, error) {
	值, 错误 := 解码MCP(记录.值)
	if 错误 != nil || 记录.键 != MCP键 || 记录.修订号 < 1 {
		return MCP{}, 错误判断包.合并(存储错误, 无效错误)
	}
	return 值, nil
}

func (当前MCP服务端 MCP服务端) 有效() bool {
	if !标识包.有效版本7(当前MCP服务端.标识) || 字符串包.裁剪空间(当前MCP服务端.名称) != 当前MCP服务端.名称 || 当前MCP服务端.名称 == "" || len(当前MCP服务端.名称) > 240 || len(当前MCP服务端.允许工具集) > 128 {
		return false
	}
	if 当前MCP服务端.范围 != "" {
		if 当前MCP服务端.传输 != MCPHTTP || 当前MCP服务端.认证 != MCPOAuth {
			return false
		}
		if _, 错误 := 规范化MCPOAuth范围(当前MCP服务端.范围); 错误 != nil {
			return false
		}
	}
	工具集 := map[string]bool{}
	for _, 名称 := range 当前MCP服务端.允许工具集 {
		if 名称 == "" || len(名称) > 256 || 字符串包.包含任一字符(名称, "\x00\r\n") || 工具集[名称] {
			return false
		}
		工具集[名称] = true
	}
	switch 当前MCP服务端.传输 {
	case MCP标准输入输出:
		if 当前MCP服务端.认证 != MCP无 || 当前MCP服务端.URL != "" || 当前MCP服务端.客户端标识 != "" || 当前MCP服务端.重定向URI != "" || !文件路径包.是否绝对路径(当前MCP服务端.命令) || 字符串包.包含字符(当前MCP服务端.命令, 0) || len(当前MCP服务端.启动参数) > 64 || len(当前MCP服务端.环境变量) > 64 || (当前MCP服务端.目录 != "" && (!文件路径包.是否绝对路径(当前MCP服务端.目录) || 字符串包.包含字符(当前MCP服务端.目录, 0))) {
			return false
		}
		for _, 参数 := range 当前MCP服务端.启动参数 {
			if 字符串包.包含字符(参数, 0) {
				return false
			}
		}
		for 键, 值 := range 当前MCP服务端.环境变量 {
			if !有效MCP环境变量(键, 值) {
				return false
			}
		}
		return true
	case MCPHTTP:
		if 地址, 错误 := MCPHTTPS(当前MCP服务端.URL); 错误 != nil || 地址.原始查询 != "" || 地址.强制查询 || 当前MCP服务端.命令 != "" || len(当前MCP服务端.启动参数) > 0 || 当前MCP服务端.目录 != "" || len(当前MCP服务端.环境变量) > 0 {
			return false
		}
		if 当前MCP服务端.认证 != MCPOAuth {
			return (当前MCP服务端.认证 == MCP无 || 当前MCP服务端.认证 == MCP词元) && 当前MCP服务端.客户端标识 == "" && 当前MCP服务端.重定向URI == ""
		}
		if len(当前MCP服务端.客户端标识) > 1024 || 字符串包.裁剪空间(当前MCP服务端.客户端标识) != 当前MCP服务端.客户端标识 || 字符串包.包含任一字符(当前MCP服务端.客户端标识, "\x00\r\n") {
			return false
		}
		if 当前MCP服务端.重定向URI == "" {
			return true
		}
		地址, 错误 := URL包.解析(当前MCP服务端.重定向URI)
		if 错误 != nil || 地址.方案 != "http" || 地址.主机名() != "127.0.0.1" || 地址.路径 == "" || 地址.用户 != nil || 地址.原始查询 != "" || 地址.强制查询 || 地址.片段 != "" {
			return false
		}
		端口, 错误 := 字符串转换包.解析无符号整数(地址.端口(), 10, 16)
		return 错误 == nil && 端口 > 0
	}
	return false
}

// 规范化MCPOAuth范围 校验权限名，并规范化空格、顺序和重复项。
func 规范化MCPOAuth范围(原始 string) (string, error) {
	if len(原始) > 1024 {
		return "", 无效错误
	}
	for _, 通道 := range 原始 {
		if 通道 < ' ' || 通道 > '~' || 通道 == '"' || 通道 == '\\' {
			return "", 无效错误
		}
	}
	值集 := 字符串包.字段集(原始)
	if len(值集) == 0 {
		return "", 无效错误
	}
	切片包.排序(值集)
	return 字符串包.合并(切片包.压缩(值集), " "), nil
}

func 有效MCP环境变量(键, 值 string) bool {
	return 环境变量名称.匹配字符串(键) && len(值) <= 4096 && !字符串包.包含字符(值, 0)
}

// MCPHTTPS 限定可配置的 HTTPS 端点，禁止凭据和片段混入地址。
func MCPHTTPS(原始 string) (*URL包.URL, error) {
	地址, 错误 := URL包.解析(原始)
	if 错误 != nil || len(原始) > 2048 || 地址.方案 != "https" || 地址.主机名() == "" || 地址.用户 != nil || 地址.片段 != "" || 地址.不透明 != "" {
		return nil, 无效错误
	}
	return 地址, nil
}

func (当前MCP MCP) 服务端(标识 string) (MCP服务端, bool) {
	索引 := 切片包.索引函数(当前MCP.服务集, func(服务端 MCP服务端) bool { return 服务端.标识 == 标识 })
	if 索引 < 0 {
		return MCP服务端{}, false
	}
	return 当前MCP.服务集[索引], true
}
```
