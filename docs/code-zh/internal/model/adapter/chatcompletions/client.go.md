```go
package 聊天补全

import (
	上下文包 "context"
	错误判断包 "errors"
	HTTP包 "net/http"
	URL包 "net/url"
	字符串包 "strings"
	同步包 "sync"

	OpenAI包 "github.com/openai/openai-go/v3"
	选项包 "github.com/openai/openai-go/v3/option"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 配置 是 对话 Completions 客户端的运行时连接配置。
type 配置 struct {
	// 基础URL 是 API 根地址，例如 https://网关.example/版本1。
	基础URL string

	// API密钥 是运行时读取的凭据。
	API密钥 string
	// 读取附件 读取并核对服务端托管的原图。
	读取附件 func(上下文包.上下文, string) ([]byte, string, error)
}

// 客户端 将 对话 Completions 协议转换为统一的模型接口。
type 客户端 struct {
	SDK   OpenAI包.对话生成服务
	读取附件  func(上下文包.上下文, string) ([]byte, string, error)
	图片互斥锁 同步包.互斥锁
	图片集   map[string]string
}

var _ 模型包.客户端 = (*客户端)(nil)

// 新建 创建模型客户端，HTTP 连接池的生命周期由调用方管理。
// 连接配置只使用显式参数，不继承 SDK 的环境变量配置。
func 新建(启动配置 配置, HTTP客户端 *HTTP包.客户端) (*客户端, error) {
	if HTTP客户端 == nil {
		return nil, 错误判断包.新建("http client 不能为空")
	}

	地址, 错误 := URL包.解析(启动配置.基础URL)
	if 错误 != nil ||
		(地址.方案 != "http" && 地址.方案 != "https") ||
		地址.主机 == "" ||
		地址.用户 != nil ||
		地址.原始查询 != "" ||
		地址.片段 != "" {
		return nil, 错误判断包.新建("API 根地址无效")
	}

	SDK := OpenAI包.新建对话生成服务(
		选项包.带有基准URL(字符串包.裁剪右(地址.字符串(), "/")+"/"),
		选项包.带有API密钥(启动配置.API密钥),
		选项包.带有HTTP客户端(HTTP客户端),

		// 重试策略由上层明确决定。
		选项包.带有最大重试集(0),
	)

	return &客户端{SDK: SDK, 读取附件: 启动配置.读取附件, 图片集: make(map[string]string)}, nil
}
```
