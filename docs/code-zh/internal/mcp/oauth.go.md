```go
package MCP

import (
	上下文包 "context"
	随机包 "crypto/rand"
	SHA256包 "crypto/sha256"
	密码辅助包 "crypto/subtle"
	基准64包 "encoding/base64"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	网络包 "net"
	HTTP包 "net/http"
	URL包 "net/url"
	切片包 "slices"
	字符串包 "strings"
	时间包 "time"

	OAuth扩展包 "github.com/modelcontextprotocol/go-sdk/oauthex"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

type OAuth状态 struct {
	状态  string `json:"status"`
	消息  string `json:"message,omitempty"`
	URL string `json:"url,omitempty"`
}

type 授权 struct {
	取消函数 上下文包.取消函数
	查看   OAuth状态 // 由 服务.互斥锁 保护。
}

type OAuth回调 struct {
	代码 string
	错误 error
}

var OAuth已拒绝错误 = 错误判断包.新建("用户拒绝了 MCP 授权")

const OAuth词元超时 = 15 * 时间包.秒

func (当前服务 *服务) 开始OAuth(执行上下文, 所属对象 上下文包.上下文, 标识 string) (OAuth状态, error) {
	当前条目, 错误 := 当前服务.获取(标识)
	if 错误 != nil {
		return OAuth状态{}, 错误
	}
	if 当前条目.启动配置.认证 != 设置包.MCPOAuth {
		return OAuth状态{}, 设置包.无效错误
	}
	执行上下文, 取消发现 := 条目上下文(执行上下文, 当前条目)
	defer 取消发现()
	元数据, 资源, 范围, 错误 := 当前服务.发现OAuth(执行上下文, 当前条目.启动配置.URL)
	if 错误 != nil {
		return OAuth状态{}, 错误
	}
	if 当前条目.启动配置.范围 != "" {
		范围 = 当前条目.启动配置.范围
	}
	URI, 监听器, 错误 := 监听回调(执行上下文, 当前条目.启动配置.重定向URI)
	if 错误 != nil {
		return OAuth状态{}, 回调错误
	}
	自有 := true
	defer func() {
		if 自有 {
			_ = 监听器.关闭()
		}
	}()
	凭据 := 凭据{客户端标识: 当前条目.启动配置.客户端标识, 词元URL: 元数据.词元端点, 资源: 资源}
	if 凭据.客户端标识 == "" {
		if 元数据.注册端点 == "" {
			return OAuth状态{}, 客户端注册错误
		}
		if _, 错误 := 设置包.MCPHTTPS(元数据.注册端点); 错误 != nil {
			return OAuth状态{}, OAuth错误
		}
		授权类型集 := []string{"authorization_code"}
		if 切片包.包含(元数据.授权类型集受支持, "refresh_token") {
			授权类型集 = append(授权类型集, "refresh_token")
		}
		注册表, 错误 := OAuth扩展包.注册客户端(执行上下文, 元数据.注册端点, &OAuth扩展包.客户端注册元数据{
			客户端名称: "Callum", 应用类型: "native", 重定向URI集: []string{URI.字符串()},
			词元端点认证方法: "none", 授权类型集: 授权类型集, 响应类型集: []string{"code"},
			范围: 当前条目.启动配置.范围,
		}, 当前服务.OAuthHTTP)
		if 错误 != nil {
			if 注册, 成功 := 错误判断包.按类型匹配[*OAuth扩展包.客户端注册错误](错误); 成功 {
				switch 注册.错误代码 {
				case "invalid_redirect_uri":
					return OAuth状态{}, 回调错误
				case "invalid_scope":
					return OAuth状态{}, OAuth范围错误
				}
			}
			return OAuth状态{}, OAuth错误
		}
		if 注册表.客户端标识 == "" || !切片包.包含(注册表.重定向URI集, URI.字符串()) || 注册表.客户端密钥 != "" {
			return OAuth状态{}, OAuth错误
		}
		凭据.客户端标识 = 注册表.客户端标识
	}
	核验器 := 基准64包.原始URL编码.编码目标字符串(随机字节())
	状态 := 基准64包.原始URL编码.编码目标字符串(随机字节())
	认证挑战 := SHA256包.校验和256([]byte(核验器))
	认证URL, _ := 设置包.MCPHTTPS(元数据.授权端点)
	查询 := 认证URL.查询()
	查询.设置("response_type", "code")
	查询.设置("client_id", 凭据.客户端标识)
	查询.设置("redirect_uri", URI.字符串())
	查询.设置("resource", 资源)
	查询.设置("state", 状态)
	查询.设置("code_challenge", 基准64包.原始URL编码.编码目标字符串(认证挑战[:]))
	查询.设置("code_challenge_method", "S256")
	if 范围 != "" {
		查询.设置("scope", 范围)
	}
	认证URL.原始查询 = 查询.编码()
	if len(认证URL.字符串()) > 8192 {
		return OAuth状态{}, OAuth错误
	}
	流程执行上下文, 取消函数 := 上下文包.创建超时上下文(当前条目.执行上下文, 5*时间包.分钟)
	停止所属对象 := 上下文包.之后函数(所属对象, 取消函数)
	流程 := &授权{取消函数: 取消函数, 查看: OAuth状态{状态: "pending"}}
	当前服务.互斥锁.加锁()
	defer 当前服务.互斥锁.解锁()
	if 当前服务.已关闭 || 当前服务.条目集[标识] != 当前条目 || 当前条目.执行上下文.获取错误() != nil || 所属对象.获取错误() != nil {
		停止所属对象()
		取消函数()
		return OAuth状态{}, 变化通知错误
	}
	if 当前条目.流程 != nil {
		当前条目.流程.取消函数()
	}
	当前条目.流程 = 流程
	当前服务.等待组.Go(func() {
		defer 停止所属对象()
		defer 取消函数()
		当前服务.结束OAuth(流程执行上下文, 当前条目, 流程, 监听器, URI, 元数据, 状态, 核验器, 凭据)
	})
	自有 = false
	return OAuth状态{状态: "pending", URL: 认证URL.字符串()}, nil
}

func 随机字节() []byte {
	输出 := make([]byte, 32)
	随机包.读取(输出)
	return 输出
}

func 监听回调(执行上下文 上下文包.上下文, 已配置 string) (*URL包.URL, 网络包.监听器, error) {
	URI := &URL包.URL{方案: "http", 主机: "127.0.0.1:0", 路径: "/oauth/callback"}
	if 已配置 != "" {
		URI, _ = URL包.解析(已配置)
	}
	监听器, 错误 := (&网络包.监听配置{}).监听(执行上下文, "tcp4", URI.主机)
	if 错误 != nil {
		return nil, nil, 错误
	}
	URI.主机 = 监听器.地址().字符串()
	return URI, 监听器, nil
}

func (当前服务 *服务) 结束OAuth(执行上下文 上下文包.上下文, 当前条目 *条目, 流程 *授权, 监听器 网络包.监听器, URI *URL包.URL, 元数据 *OAuth扩展包.认证服务端元数据, 状态, 核验器 string, 凭据 凭据) {
	结果 := make(chan OAuth回调, 1)
	操作结果 := make(chan OAuth状态, 1)
	已受理 := make(chan struct{}, 1)
	服务端 := &HTTP包.服务端{
		读取头部超时: 3 * 时间包.秒, 读取超时: 5 * 时间包.秒, 写入超时: OAuth词元超时 + 5*时间包.秒, 最大头部字节: 8192,
		处理器: HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
			当前响应写入器.头部().设置("Cache-Control", "no-store")
			当前响应写入器.头部().设置("Referrer-Policy", "no-referrer")
			if 当前请求.方法 != HTTP包.方法获取 || 当前请求.主机 != URI.主机 || 当前请求.URL.已转义路径() != URI.已转义路径() {
				HTTP包.错误文本(当前响应写入器, "无效的回调地址", HTTP包.状态未找到)
				return
			}
			查询, 错误 := URL包.解析查询(当前请求.URL.原始查询)
			if 错误 != nil || len(查询["state"]) != 1 || 密码辅助包.常量时间比较([]byte(查询.获取("state")), []byte(状态)) != 1 ||
				len(查询["iss"]) > 1 || ((查询.存在("iss") || 元数据.授权响应签发方参数受支持) && 查询.获取("iss") != 元数据.签发方) {
				写入OAuth回调页(当前响应写入器, HTTP包.状态无效请求, "", OAuth状态{状态: "failed", 消息: "授权回调校验失败。"})
				return
			}
			回调 := OAuth回调{代码: 查询.获取("code")}
			switch {
			case 查询.获取("error") == "invalid_redirect_uri" || 查询.获取("error") == "redirect_uri_mismatch":
				回调.错误 = 回调错误
			case 查询.获取("error") == "access_denied":
				回调.错误 = OAuth已拒绝错误
			case 查询.获取("error") == "invalid_scope":
				回调.错误 = OAuth范围错误
			case 查询.存在("error"), len(查询["code"]) != 1, 回调.代码 == "", len(回调.代码) > 4096:
				回调.错误 = OAuth错误
			}
			select {
			case 已受理 <- struct{}{}:
				结果 <- 回调
			default:
				写入OAuth回调页(当前响应写入器, HTTP包.状态冲突, "", OAuth状态{状态: "failed", 消息: "该授权回调已经接收。"})
				return
			}
			// 浏览器等待最终状态；关闭页面只结束本次 HTTP 响应，换令牌仍由授权任务管理。
			select {
			case 状态 := <-操作结果:
				写入OAuth回调页(当前响应写入器, HTTP包.状态成功, 当前条目.启动配置.名称, 状态)
			case <-当前请求.上下文().完成():
			}
		}),
	}
	已结束 := make(chan struct{})
	go func() {
		defer close(已结束)
		_ = 服务端.提供服务(监听器)
	}()
	var 回调 OAuth回调
	select {
	case <-执行上下文.完成():
		回调.错误 = 执行上下文.获取错误()
	case 回调 = <-结果:
	case <-已结束:
		回调.错误 = 回调错误
	}
	if 回调.错误 == nil {
		词元执行上下文, 取消函数 := 上下文包.创建超时上下文(执行上下文, OAuth词元超时)
		回调.错误 = 当前服务.完成OAuth(词元执行上下文, 当前条目, 流程, 凭据, URI.字符串(), 回调.代码, 核验器)
		取消函数()
	}
	操作结果 <- 当前服务.结束OAuth状态(当前条目, 流程, 回调.错误)

	// 停止接收新请求，并让正在发送的结果页在有界时间内完成。
	关闭执行上下文, 取消函数 := 上下文包.创建超时上下文(上下文包.无取消(执行上下文), 2*时间包.秒)
	if 服务端.关闭(关闭执行上下文) != nil {
		_ = 服务端.关闭()
	}
	取消函数()
	<-已结束
}

func (当前服务 *服务) 结束OAuth状态(当前条目 *条目, 流程 *授权, 错误 error) OAuth状态 {
	当前服务.互斥锁.加锁()
	defer 当前服务.互斥锁.解锁()
	if 当前条目.流程 != 流程 {
		return OAuth状态{状态: "cancelled", 消息: "该授权已被另一轮授权替代。"}
	}
	if 流程.查看.状态 != "pending" {
		return 流程.查看
	}
	switch {
	case 错误 == nil:
		流程.查看 = OAuth状态{状态: "authorized"}
	case 错误判断包.是否属于(错误, OAuth已拒绝错误):
		流程.查看 = OAuth状态{状态: "cancelled", 消息: "你已在服务商页面取消授权。"}
	case 错误判断包.是否属于(错误, 上下文包.已取消错误):
		流程.查看 = OAuth状态{状态: "cancelled"}
	case 错误判断包.是否属于(错误, 上下文包.超出截止时间错误):
		流程.查看 = OAuth状态{状态: "failed", 消息: "等待授权超时，请返回 Callum 后重试。"}
	default:
		流程.查看 = OAuth状态{状态: "failed", 消息: 错误.错误文本()}
	}
	return 流程.查看
}

func (当前服务 *服务) 完成OAuth(执行上下文 上下文包.上下文, 当前条目 *条目, 流程 *授权, 当前凭据 凭据, 重定向, 代码, 核验器 string) error {
	下一个, 错误 := 当前服务.词元请求(执行上下文, 当前凭据, URL包.值集{
		"grant_type": {"authorization_code"}, "code": {代码}, "redirect_uri": {重定向}, "code_verifier": {核验器},
	}, 当前条目.启动配置.范围)
	if 错误 != nil {
		return 错误
	}
	select {
	case 当前条目.词元门控 <- struct{}{}:
		defer func() { <-当前条目.词元门控 }()
	case <-执行上下文.完成():
		return 执行上下文.获取错误()
	}
	当前服务.互斥锁.加锁()
	defer 当前服务.互斥锁.解锁()
	if 执行上下文.获取错误() != nil || 当前服务.已关闭 || 当前服务.条目集[当前条目.启动配置.标识] != 当前条目 || 当前条目.流程 != 流程 {
		return 上下文包.已取消错误
	}
	if 错误 := 当前服务.仓储.保存凭据(执行上下文, 当前条目.启动配置.标识, 下一个); 错误 != nil {
		return 错误判断包.新建("无法安全保存 MCP 授权凭据")
	}
	当前条目.凭据版本.添加(1)
	// 成功状态和凭据写入在同一锁内发布，避免写入完成后的取消请求覆盖结果。
	流程.查看 = OAuth状态{状态: "authorized"}
	return nil
}

func (当前服务 *服务) OAuth状态(标识 string, 取消函数 bool) (OAuth状态, error) {
	当前服务.互斥锁.加锁()
	defer 当前服务.互斥锁.解锁()
	当前条目 := 当前服务.条目集[标识]
	if 当前条目 == nil {
		return OAuth状态{}, 未找到错误
	}
	if 当前条目.流程 == nil {
		return OAuth状态{状态: "idle"}, nil
	}
	if 取消函数 && 当前条目.流程.查看.状态 == "pending" {
		当前条目.流程.查看 = OAuth状态{状态: "cancelled"}
		当前条目.流程.取消函数()
	}
	return 当前条目.流程.查看, nil
}

func (当前服务 *服务) 发现OAuth(执行上下文 上下文包.上下文, 端点 string) (*OAuth扩展包.认证服务端元数据, string, string, error) {
	资源, _ := 设置包.MCPHTTPS(端点)
	var 候选集 []string
	范围 := ""
	// 从实际 MCP 端点读取 OAuth 认证挑战，其次尝试规范的 正确-已知 地址。
	正文 := 字符串包.新建读取器(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"Callum","version":"1.0"}}}`)
	if 请求, 错误 := HTTP包.新建请求带有上下文(执行上下文, HTTP包.方法提交, 端点, 正文); 错误 == nil {
		请求.头部.设置("Content-Type", "application/json")
		请求.头部.设置("Accept", "application/json, text/event-stream")
		if 响应, 错误 := 当前服务.HTTP.执行(请求); 错误 == nil {
			_ = 响应.正文.关闭()
			if 响应.状态代码 == HTTP包.状态未授权 {
				认证挑战集, _ := OAuth扩展包.解析网页认证(响应.头部.值集("WWW-Authenticate"))
				for _, 丙 := range 认证挑战集 {
					if 丙.方案 == "bearer" {
						范围 = 丙.参数["scope"]
						if 提示 := 丙.参数["resource_metadata"]; 提示 != "" {
							候选集 = append(候选集, 提示)
						}
						break
					}
				}
			}
		}
	}
	候选集 = append(候选集, 正确已知(资源, "/.well-known/oauth-protected-resource", true), 正确已知(资源, "/.well-known/oauth-protected-resource", false))
	for _, 候选 := range 候选集 {
		if _, 错误 := 设置包.MCPHTTPS(候选); 错误 != nil {
			continue
		}
		参数, 错误 := OAuth扩展包.获取受保护资源元数据(执行上下文, 候选, 端点, 当前服务.OAuthHTTP)
		if 错误 != nil {
			continue
		}
		for _, 标识 := range 参数.授权服务集 {
			签发方, 错误 := 设置包.MCPHTTPS(标识)
			if 错误 != nil || 签发方.原始查询 != "" {
				continue
			}
			for _, 路径 := range []string{正确已知(签发方, "/.well-known/oauth-authorization-server", true), 正确已知(签发方, "/.well-known/openid-configuration", true), 签发方.字符串() + "/.well-known/openid-configuration"} {
				元数据, 错误 := OAuth扩展包.获取认证服务端元数据(执行上下文, 路径, 标识, 当前服务.OAuthHTTP)
				if 错误 != nil || !切片包.包含(元数据.代码认证挑战方法集受支持, "S256") {
					continue
				}
				if _, 错误 := 设置包.MCPHTTPS(元数据.授权端点); 错误 != nil {
					continue
				}
				if _, 错误 := 设置包.MCPHTTPS(元数据.词元端点); 错误 != nil {
					continue
				}
				if 范围 == "" {
					范围 = 字符串包.合并(参数.范围集受支持, " ")
				}
				return 元数据, 参数.资源, 范围, nil
			}
		}
	}
	return nil, "", "", OAuth错误
}

func 正确已知(来源 *URL包.URL, 前缀 string, 包含路径 bool) string {
	地址 := URL包.URL{方案: 来源.方案, 主机: 来源.主机, 路径: 前缀}
	if 包含路径 {
		地址.路径 += 字符串包.裁剪右(来源.路径, "/")
	}
	return 地址.字符串()
}

func (当前服务 *服务) 词元请求(执行上下文 上下文包.上下文, 当前凭据 凭据, 表单 URL包.值集, 范围 string) (凭据, error) {
	表单.设置("client_id", 当前凭据.客户端标识)
	表单.设置("resource", 当前凭据.资源)
	请求, 错误 := HTTP包.新建请求带有上下文(执行上下文, HTTP包.方法提交, 当前凭据.词元URL, 字符串包.新建读取器(表单.编码()))
	if 错误 != nil {
		return 凭据{}, OAuth错误
	}
	请求.头部.设置("Content-Type", "application/x-www-form-urlencoded")
	请求.头部.设置("Accept", "application/json")
	响应, 错误 := 当前服务.OAuthHTTP.执行(请求)
	if 错误 != nil {
		if 执行上下文.获取错误() != nil {
			return 凭据{}, 执行上下文.获取错误()
		}
		return 凭据{}, OAuth错误
	}
	defer 响应.正文.关闭()
	var 结果 struct {
		附件访问器 string         `json:"access_token"`
		类型    string         `json:"token_type"`
		刷新    string         `json:"refresh_token"`
		过期时间  int64          `json:"expires_in"`
		错误    string         `json:"error"`
		范围    JSON包.原始JSON消息 `json:"scope"`
	}
	解码器 := JSON包.新建解码器(上限正文(响应.正文))
	if 解码器.解码(&结果) != nil || 解码器.解码(new(any)) != 输入输出包.文件结束 {
		return 凭据{}, OAuth错误
	}
	switch 结果.错误 {
	case "invalid_grant", "invalid_client":
		return 凭据{}, 授权错误
	case "invalid_redirect_uri", "redirect_uri_mismatch":
		return 凭据{}, 回调错误
	case "invalid_scope":
		return 凭据{}, OAuth范围错误
	}
	if 响应.状态代码 != HTTP包.状态成功 || 结果.错误 != "" || !字符串包.相等折叠(结果.类型, "Bearer") ||
		len(结果.附件访问器) > 8192 || len(结果.刷新) > 8192 || 结果.过期时间 < 0 || 结果.过期时间 > 1<<31-1 ||
		结果.附件访问器 == "" || 字符串包.包含任一字符(结果.附件访问器, "\x00\r\n \t") {
		return 凭据{}, OAuth错误
	}
	// RFC 6749 允许服务商在授予的范围与请求一致时省略 范围。
	if 范围 != "" && 结果.范围 != nil {
		var 已授权 string
		if JSON包.反序列化(结果.范围, &已授权) != nil {
			return 凭据{}, OAuth范围不匹配错误
		}
		if 已规范化, 错误 := 设置包.规范化MCPOAuth范围(已授权); 错误 != nil || 已规范化 != 范围 {
			return 凭据{}, OAuth范围不匹配错误
		}
	}
	当前凭据.词元 = 结果.附件访问器
	当前凭据.过期时间 = 时间包.时间点{}
	if 结果.刷新 != "" {
		当前凭据.刷新 = 结果.刷新
	}
	if 结果.过期时间 > 0 {
		当前凭据.过期时间 = 时间包.当前时间().添加(时间包.时长(结果.过期时间) * 时间包.秒)
	}
	return 当前凭据, nil
}

func (当前服务 *服务) 访问词元(执行上下文 上下文包.上下文, 当前条目 *条目) (string, error) {
	select {
	case 当前条目.词元门控 <- struct{}{}:
		defer func() { <-当前条目.词元门控 }()
	case <-执行上下文.完成():
		return "", 执行上下文.获取错误()
	case <-当前条目.执行上下文.完成():
		return "", 变化通知错误
	}
	丙, 错误 := 当前服务.仓储.凭据(执行上下文, 当前条目.启动配置.标识)
	if 错误 != nil {
		return "", 错误
	}
	if 丙.词元 == "" {
		return "", 授权错误
	}
	if 丙.过期时间.是否零值() || 时间包.直到(丙.过期时间) > 30*时间包.秒 {
		return 丙.词元, nil
	}
	if 丙.刷新 == "" {
		if 时间包.直到(丙.过期时间) > 0 {
			return 丙.词元, nil
		}
		return "", 授权错误
	}
	词元执行上下文, 取消函数 := 上下文包.创建超时上下文(执行上下文, 10*时间包.秒)
	defer 取消函数()
	表单 := URL包.值集{"grant_type": {"refresh_token"}, "refresh_token": {丙.刷新}}
	if 当前条目.启动配置.范围 != "" {
		表单.设置("scope", 当前条目.启动配置.范围)
	}
	下一个, 错误 := 当前服务.词元请求(词元执行上下文, 丙, 表单, 当前条目.启动配置.范围)
	if 错误 != nil && !错误判断包.是否属于(错误, 授权错误) && !错误判断包.是否属于(错误, OAuth范围错误) && !错误判断包.是否属于(错误, OAuth范围不匹配错误) {
		return "", 错误
	}
	当前服务.互斥锁.加锁()
	defer 当前服务.互斥锁.解锁()
	if 当前服务.已关闭 || 当前服务.条目集[当前条目.启动配置.标识] != 当前条目 || 当前条目.执行上下文.获取错误() != nil {
		return "", 变化通知错误
	}
	最新, 加载错误 := 当前服务.仓储.凭据(执行上下文, 当前条目.启动配置.标识)
	if 加载错误 != nil {
		return "", 加载错误
	}
	if 最新.词元 != 丙.词元 || 最新.刷新 != 丙.刷新 {
		return 最新.词元, nil
	}
	if 写入错误 := 当前服务.仓储.保存凭据(执行上下文, 当前条目.启动配置.标识, 下一个); 写入错误 != nil {
		return "", 写入错误
	}
	if 错误 != nil {
		// 凭据失效后，旧连接和已发现的工具随之失效。
		当前条目.凭据版本.添加(1)
	}
	return 下一个.词元, 错误
}
```
