```go
package MCP测试

import (
	上下文包 "context"
	SHA256包 "crypto/sha256"
	基准64包 "encoding/base64"
	JSON包 "encoding/json"
	输入输出包 "io"
	结构化日志包 "log/slog"
	网络包 "net"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	URL包 "net/url"
	文件路径包 "path/filepath"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"
	时间包 "time"

	SDK包 "github.com/modelcontextprotocol/go-sdk/mcp"
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	凭据测试包 "github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
	MCP包 "github.com/xiaoyu98628/callum/internal/mcp"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	MCP仓储包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/mcp"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test可流式HTTPOAuth使用回环并且已加密存储(测试器 *测试包.测试器) {
	for _, 固定 := range []bool{false, true} {
		名称 := "random"
		if 固定 {
			名称 = "fixed"
		}
		测试器.执行(名称, func(测试器 *测试包.测试器) {
			测试可流式OAuth(测试器, 固定)
		})
	}
}

func 测试可流式OAuth(测试器 *测试包.测试器, 固定 bool) {
	执行上下文 := 测试器.上下文()
	const 访问 = "controlled-access-token"
	const 刷新 = "controlled-refresh-token"
	const 已轮换 = "controlled-rotated-access-token"
	const 非预期 = "controlled-scope-mismatch-token"
	const 声明的范围 = "read write"
	var 已配置范围 string
	if !固定 {
		已配置范围 = "read"
	}
	预期范围 := 已配置范围
	if 预期范围 == "" {
		预期范围 = 声明的范围
	}
	var 词元, 工具调用集, 注册集 原子操作包.整数32
	var 无效刷新范围 原子操作包.布尔
	var 认证认证挑战 原子操作包.值
	词元请求的 := make(chan struct{}, 1)
	释放词元 := make(chan struct{})

	SDK服务端 := SDK包.新建服务端(&SDK包.实现{名称: "回显服务", 版本: "1"}, nil)
	SDK服务端.添加工具(&SDK包.工具{名称: "echo", 说明: "受控工具", 输入表结构: JSON包.原始JSON消息(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}, func(_ 上下文包.上下文, 当前调用工具请求 *SDK包.调用工具请求) (*SDK包.调用工具结果, error) {
		工具调用集.添加(1)
		return &SDK包.调用工具结果{内容: []SDK包.内容{&SDK包.文本内容{文本: "mcp-http-ok"}}}, nil
	})
	MCP处理器 := SDK包.新建可流式HTTP处理器(func(*HTTP包.请求) *SDK包.服务端 { return SDK服务端 }, &SDK包.可流式HTTP选项集{JSON响应: 固定})
	路由器 := HTTP包.新建提供服务路由器()
	var 签发方 string
	路由器.句柄函数("/mcp", func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		if 当前请求.头部.获取("Authorization") != "Bearer "+访问 && 当前请求.头部.获取("Authorization") != "Bearer "+已轮换 {
			当前响应写入器.头部().设置("WWW-Authenticate", `Bearer resource_metadata="`+签发方+`/.well-known/oauth-protected-resource/mcp", scope="`+声明的范围+`"`)
			当前响应写入器.写入头部(HTTP包.状态未授权)
			return
		}
		// 模拟 HTTPS 名称通过受控拨号落到临时回环地址；服务端匹配实际本机 主机。
		当前请求.主机 = 当前请求.上下文().值(HTTP包.本地地址上下文键).(网络包.地址).字符串()
		MCP处理器.提供服务HTTP(当前响应写入器, 当前请求)
	})
	路由器.句柄函数("/.well-known/oauth-protected-resource/mcp", func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		写入JSON(测试器, 当前响应写入器, map[string]any{"resource": 签发方 + "/mcp", "authorization_servers": []string{签发方}})
	})
	路由器.句柄函数("/.well-known/oauth-authorization-server", func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		写入JSON(测试器, 当前响应写入器, map[string]any{"issuer": 签发方, "authorization_endpoint": 签发方 + "/authorize", "token_endpoint": 签发方 + "/token", "registration_endpoint": 签发方 + "/register", "code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true})
	})
	路由器.句柄函数("/register", func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		注册集.添加(1)
		var 输入 struct {
			重定向URI集 []string `json:"redirect_uris"`
			认证      string   `json:"token_endpoint_auth_method"`
			范围      string   `json:"scope"`
		}
		if 当前请求.方法 != HTTP包.方法提交 || JSON包.新建解码器(当前请求.正文).解码(&输入) != nil || len(输入.重定向URI集) != 1 || 输入.认证 != "none" || 输入.范围 != 已配置范围 {
			测试器.错误文本("OAuth 动态注册参数错误")
			当前响应写入器.写入头部(400)
			return
		}
		当前响应写入器.头部().设置("Content-Type", "application/json")
		当前响应写入器.写入头部(HTTP包.状态已创建)
		_ = JSON包.新建编码器(当前响应写入器).编码(map[string]any{"client_id": "test-client", "redirect_uris": 输入.重定向URI集, "token_endpoint_auth_method": "none"})
	})
	路由器.句柄函数("/token", func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		词元.添加(1)
		if 当前请求.方法 != HTTP包.方法提交 || 当前请求.解析表单() != nil || 当前请求.表单.获取("client_id") != "test-client" || 当前请求.表单.获取("resource") != 签发方+"/mcp" {
			测试器.错误文本("令牌参数无效")
			当前响应写入器.写入头部(400)
			return
		}
		if 当前请求.表单.获取("grant_type") == "refresh_token" && 当前请求.表单.获取("refresh_token") == 刷新 {
			if 当前请求.表单.获取("scope") != 已配置范围 || 当前请求.表单.存在("scope") != (已配置范围 != "") {
				测试器.错误文本("刷新请求的 scope 与配置不符")
			}
			if 无效刷新范围.加载() {
				写入JSON(测试器, 当前响应写入器, map[string]any{"access_token": 非预期, "token_type": "Bearer", "scope": 声明的范围})
				return
			}
			写入JSON(测试器, 当前响应写入器, map[string]any{"access_token": 已轮换, "token_type": "Bearer", "expires_in": 3600})
			return
		}
		switch 当前请求.表单.获取("code") {
		case "controlled-failed-code":
			当前响应写入器.写入头部(HTTP包.状态服务不可用)
			return
		case "controlled-broad-scope-code":
			写入JSON(测试器, 当前响应写入器, map[string]any{"access_token": 非预期, "token_type": "Bearer", "scope": 声明的范围})
			return
		}
		预期 := 认证认证挑战.加载().(URL包.值集)
		哈希 := SHA256包.校验和256([]byte(当前请求.表单.获取("code_verifier")))
		if 当前请求.表单.获取("grant_type") != "authorization_code" || 当前请求.表单.获取("code") != "controlled-code" ||
			当前请求.表单.获取("redirect_uri") != 预期.获取("redirect_uri") ||
			基准64包.原始URL编码.编码目标字符串(哈希[:]) != 预期.获取("code_challenge") {
			测试器.错误文本("授权码、回调地址或 PKCE 与发起请求不符")
		}
		词元请求的 <- struct{}{}
		select {
		case <-当前请求.上下文().完成():
			return
		case <-释放词元:
		}
		响应 := map[string]any{"access_token": 访问, "token_type": "Bearer", "refresh_token": 刷新, "expires_in": 3600}
		if 已配置范围 != "" {
			响应["scope"] = 已配置范围
		}
		写入JSON(测试器, 当前响应写入器, 响应)
	})
	服务端 := HTTP测试包.新建未开始服务端(路由器)
	服务端.开始TLS()
	测试器.注册清理函数(服务端.关闭)
	签发方 = 字符串包.替换(服务端.URL, "127.0.0.1", "example.com", 1)
	客户端 := 服务端.客户端()
	传输模块 := 客户端.传输.(*HTTP包.传输)
	传输模块.拨号上下文 = func(执行上下文 上下文包.上下文, 网络, _ string) (网络包.连接, error) {
		return (&网络包.拨号器{}).拨号上下文(执行上下文, 网络, 服务端.监听器.地址().字符串())
	}
	路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
	数据库, 错误 := SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() { _ = 数据库.关闭() })
	凭据保险库, 错误 := 凭据包.新建(凭据测试包.系统凭据库{}, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() { _ = 凭据保险库.关闭() })
	仓储 := MCP仓储包.新建(数据库.数据库, 凭据保险库)
	服务, 错误 := MCP包.新建(执行上下文, 执行上下文, 仓储, 客户端, 客户端, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(服务.关闭)
	启动配置 := 设置包.MCP服务端{名称: "OAuth 服务", 传输: 设置包.MCPHTTP, 认证: 设置包.MCPOAuth, URL: 签发方 + "/mcp",
		已启用: true, 允许工具集: []string{"echo"}}
	if 固定 {
		启动配置.客户端标识 = "test-client"
	} else {
		启动配置.范围 = 已配置范围
	}
	if 固定 {
		监听器, 错误 := 网络包.监听("tcp4", "127.0.0.1:0")
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		启动配置.重定向URI = "http://" + 监听器.地址().字符串() + "/return"
		标识, 错误 := 服务.保存(执行上下文, 1, 启动配置, MCP包.密钥输入{}, MCP包.保存选项集{})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if _, 错误 = 服务.开始OAuth(执行上下文, 执行上下文, 标识); 错误 != MCP包.回调错误 {
			测试器.格式化报告致命错误("占用的固定端口: %v", 错误)
		}
		if 错误 := 监听器.关闭(); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		启动配置.标识 = 标识
	}
	修订号 := int64(1)
	if 固定 {
		修订号++
	}
	标识, 错误 := 服务.保存(执行上下文, 修订号, 启动配置, MCP包.密钥输入{}, MCP包.保存选项集{})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 固定 {
		if _, 错误 := 服务.开始OAuth(执行上下文, 执行上下文, 标识); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		状态, 错误 := 服务.OAuth状态(标识, true)
		if 错误 != nil || 状态.状态 != "cancelled" {
			测试器.格式化报告致命错误("取消授权失败: %+v %v", 状态, 错误)
		}
		地址, _ := URL包.解析(启动配置.重定向URI)
		for 截止时间 := 时间包.当前时间().添加(2 * 时间包.秒); ; {
			监听器, 错误 := 网络包.监听("tcp4", 地址.主机)
			if 错误 == nil {
				_ = 监听器.关闭()
				break
			}
			if 时间包.当前时间().之后(截止时间) {
				测试器.报告致命错误("取消后仍占用回调端口")
			}
			时间包.休眠(10 * 时间包.毫秒)
		}
	}
	状态, 错误 := 服务.开始OAuth(执行上下文, 执行上下文, 标识)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if (注册集.加载() == 1) == 固定 {
		测试器.报告致命错误("OAuth 没有按服务配置使用预注册或动态登记")
	}
	授权, _ := URL包.解析(状态.URL)
	查询 := 授权.查询()
	回调, _ := URL包.解析(查询.获取("redirect_uri"))
	if 回调.主机名() != "127.0.0.1" || 回调.端口() == "" || 回调.端口() == "0" ||
		(固定 && 回调.字符串() != 启动配置.重定向URI) || 查询.获取("code_challenge_method") != "S256" || 查询.获取("scope") != 预期范围 {
		测试器.报告致命错误("授权地址的回调、PKCE 或 scope 参数错误")
	}
	认证认证挑战.存储(查询)
	浏览器 := HTTP包.客户端{超时: 2 * 时间包.秒}
	回调.原始查询 = URL包.值集{"state": {"wrong-state"}, "code": {"controlled-code"}, "iss": {签发方}}.编码()
	无效, 错误 := 浏览器.获取(回调.字符串())
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	检查OAuth页(测试器, 无效, HTTP包.状态无效请求, "failed", "授权回调校验失败", "wrong-state", "controlled-code")
	if 词元.加载() != 0 {
		测试器.报告致命错误("无效 state 触发了换令牌")
	}

	回调.原始查询 = URL包.值集{"state": {查询.获取("state")}, "code": {"controlled-code"}, "iss": {签发方}}.编码()
	type 浏览器结果 struct {
		响应 *HTTP包.响应
		错误 error
	}
	回复 := make(chan 浏览器结果, 1)
	go func() {
		响应, 错误 := 浏览器.获取(回调.字符串())
		回复 <- 浏览器结果{响应, 错误}
	}()
	select {
	case <-词元请求的:
	case <-时间包.之后(2 * 时间包.秒):
		close(释放词元)
		测试器.报告致命错误("浏览器回调未开始换取令牌")
	}
	待处理, 错误 := 服务.OAuth状态(标识, false)
	if 错误 != nil || 待处理.状态 != "pending" {
		close(释放词元)
		测试器.格式化报告致命错误("换令牌期间状态不正确: %+v %v", 待处理, 错误)
	}
	// 模拟授权端仍在处理时，浏览器不能收到声称登录成功的页面。
	select {
	case 提前 := <-回复:
		close(释放词元)
		if 提前.响应 != nil {
			_ = 提前.响应.正文.关闭()
		}
		测试器.格式化报告致命错误("换令牌完成前浏览器已经收到响应: %v", 提前.错误)
	default:
		close(释放词元)
	}
	浏览器回复 := <-回复
	if 浏览器回复.错误 != nil {
		测试器.报告致命错误(浏览器回复.错误)
	}
	检查OAuth页(测试器, 浏览器回复.响应, HTTP包.状态成功, "authorized", "授权成功", 查询.获取("state"), "controlled-code", 访问)
	状态, 错误 := 服务.OAuth状态(标识, false)
	if 错误 != nil || 状态.状态 != "authorized" {
		测试器.格式化报告致命错误("浏览器和桌面端授权状态不一致: %+v %v", 状态, 错误)
	}

	已存储, 错误 := 仓储.凭据(执行上下文, 标识)
	if 错误 != nil || 已存储.词元 != 访问 || 已存储.刷新 != 刷新 {
		测试器.格式化报告致命错误("凭据未正确保存: %v", 错误)
	}
	var 行 表结构包.MCP凭据
	if 错误 := 数据库.数据库.首个(&行).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 字符串包.包含(行.密文, 访问) || 字符串包.包含(行.密文, 刷新) || !字符串包.存在前缀(行.密文, "v1:") {
		测试器.报告致命错误("凭据未以密文保存")
	}
	目录管理, 错误 := 服务.列表(执行上下文)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	公开, _ := JSON包.序列化(目录管理)
	if 字符串包.包含(string(公开), 访问) || !目录管理.服务集[0].已授权 || 目录管理.服务集[0].范围 != 已配置范围 {
		测试器.报告致命错误("公开目录泄漏凭据或授权状态有误")
	}
	已存储.过期时间 = 时间包.当前时间().添加(-时间包.秒)
	if 错误 := 仓储.保存凭据(执行上下文, 标识, 已存储); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	列表, 错误 := 服务.测试(执行上下文, 标识, false, 修订号+1)
	if 错误 != nil || len(列表) != 1 || 列表[0].名称 != "echo" {
		测试器.格式化报告致命错误("发现失败: %+v %v", 列表, 错误)
	}
	工具集 := 服务.工具集快照(执行上下文, true, 修订号+1)
	if len(工具集) != 1 {
		测试器.报告致命错误("允许的工具未进入运行快照")
	}
	有效, 错误 := 工具集[0].校验(JSON包.原始JSON消息(`{"value":"sample"}`))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	工具调用, 错误 := 工具集[0].准备(执行上下文, 工具包.范围{}, 有效)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	测试器.注册清理函数(func() { _ = 工具调用.关闭() })
	结果, 错误 := 工具调用.执行(执行上下文)
	if 错误 != nil || 工具调用集.加载() != 1 || !字符串包.包含(string(结果.输出), "mcp-http-ok") || 词元.加载() != 2 {
		测试器.格式化报告致命错误("HTTP 工具或刷新请求未完成: %+v %v", 结果, 错误)
	}
	if !固定 {
		for _, 步骤 := range []struct {
			参数 URL包.值集
			状态 string
			消息 string
		}{
			{URL包.值集{"error": {"access_denied"}, "error_description": {"remote-secret-error"}}, "cancelled", "已取消授权"},
			{URL包.值集{"code": {"controlled-failed-code"}}, "failed", "授权失败"},
			{URL包.值集{"error": {"invalid_scope"}}, "failed", MCP包.OAuth范围错误.错误文本()},
			{URL包.值集{"code": {"controlled-broad-scope-code"}}, "failed", MCP包.OAuth范围不匹配错误.错误文本()},
		} {
			登录, 错误 := 服务.开始OAuth(执行上下文, 执行上下文, 标识)
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			认证URL, _ := URL包.解析(登录.URL)
			重定向, _ := URL包.解析(认证URL.查询().获取("redirect_uri"))
			回调状态 := 认证URL.查询().获取("state")
			步骤.参数.设置("state", 回调状态)
			步骤.参数.设置("iss", 签发方)
			重定向.原始查询 = 步骤.参数.编码()
			结果, 错误 := 浏览器.获取(重定向.字符串())
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			检查OAuth页(测试器, 结果, HTTP包.状态成功, 步骤.状态, 步骤.消息, 回调状态, "remote-secret-error", 步骤.参数.获取("code"), 非预期)
			状态, 错误 := 服务.OAuth状态(标识, false)
			if 错误 != nil || 状态.状态 != 步骤.状态 {
				测试器.格式化报告致命错误("浏览器和桌面端授权状态不一致: %+v %v", 状态, 错误)
			}
		}
		if 词元.加载() != 4 {
			测试器.格式化报告致命错误("授权拒绝不应访问换令牌接口，实际令牌请求数: %d", 词元.加载())
		}
		已刷新, 错误 := 仓储.凭据(执行上下文, 标识)
		if 错误 != nil || 已刷新.词元 != 已轮换 {
			测试器.报告致命错误("拒绝范围不符的授权后，已保存的凭据应继续有效")
		}
		启动配置.标识, 启动配置.范围 = 标识, " read   read "
		if _, 错误 := 服务.保存(执行上下文, 修订号+1, 启动配置, MCP包.密钥输入{}, MCP包.保存选项集{}); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		修订号++
		if 目录管理, 错误 := 服务.列表(执行上下文); 错误 != nil || !目录管理.服务集[0].已授权 || 目录管理.服务集[0].范围 != "read" || len(目录管理.服务集[0].允许工具集) != 1 {
			测试器.格式化报告致命错误("规范化后的权限相同，应保留凭据和工具选择: %v", 错误)
		}
		启动配置.范围 = "" // 旧客户端更新同一服务时省略 范围。
		if _, 错误 := 服务.保存(执行上下文, 修订号+1, 启动配置, MCP包.密钥输入{}, MCP包.保存选项集{}); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		修订号++
		if 目录管理, 错误 := 服务.列表(执行上下文); 错误 != nil || !目录管理.范围更新 || !目录管理.服务集[0].已授权 || 目录管理.服务集[0].范围 != "read" || len(目录管理.服务集[0].允许工具集) != 1 {
			测试器.格式化报告致命错误("旧客户端省略 scope 应保留范围、凭据和工具选择: %v", 错误)
		}
		if _, 错误 := 服务.测试(执行上下文, 标识, false, 修订号+1); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		已过期 := 已刷新
		已过期.过期时间 = 时间包.当前时间().添加(-时间包.秒)
		if 错误 := 仓储.保存凭据(执行上下文, 标识, 已过期); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		无效刷新范围.存储(true)
		if _, 错误 := 服务.测试(执行上下文, 标识, false, 修订号+1); 错误 != MCP包.OAuth范围不匹配错误 {
			测试器.格式化报告致命错误("刷新后权限范围不一致应返回明确错误: %v", 错误)
		}
		if 已保存, 错误 := 仓储.凭据(执行上下文, 标识); 错误 != nil || !已保存.空() {
			测试器.格式化报告致命错误("刷新失败应清除失效凭据: %v", 错误)
		}
		if 错误 := 仓储.保存凭据(执行上下文, 标识, 已刷新); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		启动配置.范围 = "read profile"
		if _, 错误 := 服务.保存(执行上下文, 修订号+1, 启动配置, MCP包.密钥输入{}, MCP包.保存选项集{}); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		修订号++
		if 目录管理, 错误 := 服务.列表(执行上下文); 错误 != nil || 目录管理.服务集[0].已授权 || 目录管理.服务集[0].范围 != "profile read" || len(目录管理.服务集[0].允许工具集) != 0 {
			测试器.格式化报告致命错误("权限范围变更后应清除原凭据和工具选择: %v", 错误)
		}
		if 错误 := 仓储.保存凭据(执行上下文, 标识, 已刷新); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		启动配置.范围 = ""
		if _, 错误 := 服务.保存(执行上下文, 修订号+1, 启动配置, MCP包.密钥输入{}, MCP包.保存选项集{清空范围: true}); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		修订号++
		if 目录管理, 错误 := 服务.列表(执行上下文); 错误 != nil || 目录管理.服务集[0].已授权 || 目录管理.服务集[0].范围 != "" || len(目录管理.服务集[0].允许工具集) != 0 {
			测试器.格式化报告致命错误("显式清除 scope 应恢复自动发现并清除旧授权: %v", 错误)
		}
	}
	if 错误 := 服务.删除(执行上下文, 修订号+1, 标识); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已移除, 错误 := 仓储.凭据(执行上下文, 标识)
	if 错误 != nil || !已移除.空() {
		测试器.报告致命错误("删除服务没有同步清除凭据")
	}
	if 固定 {
		启动配置.标识, 启动配置.认证, 启动配置.重定向URI, 启动配置.客户端标识 = "", 设置包.MCP词元, "", ""
		手动标识, 错误 := 服务.保存(执行上下文, 修订号+2, 启动配置, MCP包.密钥输入{词元: new(访问)}, MCP包.保存选项集{})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 工具集, 错误 := 服务.测试(执行上下文, 手动标识, false, 修订号+3); 错误 != nil || len(工具集) != 1 {
			测试器.格式化报告致命错误("手填访问令牌无法连接: %v", 错误)
		}
		if 错误 := 服务.断开连接(执行上下文, 手动标识); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 凭据, 错误 := 仓储.凭据(执行上下文, 手动标识); 错误 != nil || !凭据.空() {
			测试器.报告致命错误("清除访问凭据未生效")
		}
	}

}

func 检查OAuth页(测试器 *测试包.测试器, 响应 *HTTP包.响应, 代码 int, 状态, 标题 string, 敏感 ...string) {
	测试器.标记测试辅助函数()
	defer 响应.正文.关闭()
	正文, 错误 := 输入输出包.读取全部(响应.正文)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	页 := string(正文)
	if 响应.状态代码 != 代码 || 响应.头部.获取("Content-Type") != "text/html; charset=utf-8" ||
		响应.头部.获取("Cache-Control") != "no-store" || 响应.头部.获取("Referrer-Policy") != "no-referrer" ||
		!字符串包.包含(页, "data-status=\""+状态+"\"") || !字符串包.包含(页, 标题) {
		测试器.格式化报告致命错误("授权结果页与预期不符: %d %v", 响应.状态代码, 响应.头部)
	}
	_, 随机数并且策略, 成功 := 字符串包.截断(响应.头部.获取("Content-Security-Policy"), "script-src 'nonce-")
	随机数, _, 已结束 := 字符串包.截断(随机数并且策略, "'")
	if !成功 || !已结束 || len(随机数) == 0 || !字符串包.包含(页, "<script nonce=\""+随机数+"\">") ||
		!字符串包.包含(页, "history.replaceState") {
		测试器.报告致命错误("缺少清理地址栏参数的受信脚本")
	}
	for _, 密钥 := range 敏感 {
		if 密钥 != "" && 字符串包.包含(页, 密钥) {
			测试器.报告致命错误("授权结果页包含回调参数或凭据")
		}
	}
}

func 写入JSON(测试器 *测试包.测试器, 当前响应写入器 HTTP包.响应写入器, 值 any) {
	测试器.标记测试辅助函数()
	当前响应写入器.头部().设置("Content-Type", "application/json")
	if 错误 := JSON包.新建编码器(当前响应写入器).编码(值); 错误 != nil {
		测试器.错误文本(错误)
	}
}
```
