```go
package 聊天补全

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	HTTP包 "net/http"
	同步包 "sync"

	选项包 "github.com/openai/openai-go/v3/option"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 生成 返回文本响应；错误时结果可能包含尚未完整生成的文本。
func (当前客户端 *客户端) 生成(执行上下文 上下文包.上下文, 请求 模型包.请求) (结果 模型包.响应, 返回错误 error) {
	参数, 错误 := 当前客户端.构建请求(执行上下文, 请求)
	if 错误 != nil {
		return 模型包.响应{}, 错误
	}

	var HTTP响应 *HTTP包.响应
	var 正文 *响应正文

	defer func() {
		if 正文 != nil {
			if 错误 := 正文.关闭(); 错误 != nil {
				返回错误 = 错误判断包.合并(返回错误, 包装错误("close response", HTTP响应, 错误))
			}
		}
	}()

	生成, 错误 := 当前客户端.SDK.新建(
		执行上下文,
		参数,
		选项包.带有响应进入(&HTTP响应),
		选项包.带有中间件(func(请求 *HTTP包.请求, 下一个 选项包.中间件下一个) (*HTTP包.响应, error) {
			响应, 错误 := 下一个(请求)
			// 请求失败时响应体由 HTTP 客户端处理，只接管成功取得的响应。
			if 错误 == nil && 响应 != nil && 响应.正文 != nil {
				正文 = &响应正文{读取关闭器: 响应.正文}
				响应.正文 = 正文
			}
			return 响应, 错误
		}),
	)
	if 错误 != nil {
		return 模型包.响应{}, 包装错误("generate", HTTP响应, 错误)
	}
	if 生成 != nil {
		结果.请求标识 = 请求标识(HTTP响应)
		结果.输出, 错误 = 已捕获输出("json_response", JSON包.原始JSON消息(生成.原始JSON()))
		if 错误 != nil {
			return 结果, 包装错误("decode response", HTTP响应, 错误)
		}
		结果.用量, 错误 = 捕获用量(JSON包.原始JSON消息(生成.JSON.用量.原始()))
		if 错误 != nil {
			return 结果, 包装错误("decode usage", HTTP响应, 错误)
		}
		结果.输入词元 = 输入词元(结果.用量)
	}

	if 生成 == nil || len(生成.选项集) != 1 {
		return 结果, 包装错误(
			"decode response",
			HTTP响应,
			模型包.无效响应错误,
		)
	}

	选项 := 生成.选项集[0]
	if 错误 := 校验选项(选项.原始JSON()); 错误 != nil {
		return 结果, 包装错误("decode response", HTTP响应, 错误)
	}
	if 错误 := 校验文本输出(选项.消息.原始JSON()); 错误 != nil {
		return 结果, 包装错误("decode response", HTTP响应, 错误)
	}

	结果.推理, 错误 = 推理文本(选项.消息.原始JSON())
	if 错误 != nil {
		return 结果, 包装错误("decode reasoning", HTTP响应, 错误)
	}
	结果.文本 = 选项.消息.内容
	结果.内容 = []模型包.内容块{{类型: "text", 文本: 结果.文本}}

	if 选项.消息.拒绝 != "" {
		return 结果, 包装错误(
			"decode response",
			HTTP响应,
			模型包.已拒绝错误,
		)
	}

	结果.结束原因 = 选项.结束原因
	结果.工具调用集, 错误 = 响应调用集(选项.消息.原始JSON(), 选项.结束原因, len(请求.工具集) > 0)
	if 错误 != nil {
		return 结果, 包装错误("decode response", HTTP响应, 错误)
	}

	if 错误 := 检查工具结束原因(选项.结束原因, len(请求.工具集) > 0); 错误 != nil {
		// 返回已经收到的文本，同时明确标记调用未成功完成。
		return 结果, 包装错误("finish response", HTTP响应, 错误)
	}
	if !选项.消息.JSON.内容.有效() && len(结果.工具调用集) == 0 {
		return 结果, 包装错误("decode response", HTTP响应, 模型包.无效响应错误)
	}

	return 结果, nil
}

// 响应正文 保存 SDK 已执行的关闭结果，供 生成 检查清理错误。
// SDK 和 生成 都可能调用 关闭，但底层响应体只能关闭一次。
type 响应正文 struct {
	输入输出包.读取关闭器
	关闭一次 同步包.一次
	关闭错误 error
}

func (当前响应正文 *响应正文) 关闭() error {
	当前响应正文.关闭一次.执行(func() {
		当前响应正文.关闭错误 = 当前响应正文.读取关闭器.关闭()
	})
	return 当前响应正文.关闭错误
}

// SDK 的宽松解码可能把数字转换成字符串；使用原始 JSON 校验实际消费的字段。
func 校验选项(原始 string) error {
	var 选项 struct {
		索引   *int64  `json:"index"`
		结束原因 *string `json:"finish_reason"`
	}
	if 错误 := JSON包.反序列化([]byte(原始), &选项); 错误 != nil || 选项.索引 == nil || *选项.索引 != 0 {
		return 模型包.无效响应错误
	}
	return nil
}

func 校验文本输出(原始 string) error {
	var 输出 *struct {
		内容    *string          `json:"content"`
		拒绝    *string          `json:"refusal"`
		工具调用集 []JSON包.原始JSON消息 `json:"tool_calls"`
		音频    *JSON包.原始JSON消息  `json:"audio"`
	}
	if 错误 := JSON包.反序列化([]byte(原始), &输出); 错误 != nil || 输出 == nil {
		return 模型包.无效响应错误
	}
	// 已知但尚未支持的生成内容必须明确拒绝；未知厂商元数据继续保留在原始结果中。
	if 输出.音频 != nil {
		return 模型包.不支持输出错误
	}
	return nil
}

func 检查结束原因(原因 string) error {
	switch 原因 {
	case "stop":
		return nil
	case "":
		return 输入输出包.非预期文件结束错误
	case "length":
		return 模型包.不完整错误
	case "content_filter":
		return 模型包.已拒绝错误
	case "tool_calls", "function_call":
		// 当前只支持文本，旧结束标识也仅用于拒绝输出。
		return 模型包.不支持输出错误
	default:
		return 模型包.无效响应错误
	}
}

// 推理文本 仅识别已核实的 对话 Completions 扩展，不从正文标签推测推理。
func 推理文本(原始 string) (*string, error) {
	var 消息 map[string]JSON包.原始JSON消息
	if JSON包.反序列化([]byte(原始), &消息) != nil {
		return nil, 模型包.无效响应错误
	}
	值, 成功 := 消息["reasoning_content"]
	if !成功 || string(值) == "null" {
		return nil, nil
	}
	var 文本 string
	if JSON包.反序列化(值, &文本) != nil {
		return nil, 模型包.无效响应错误
	}
	return &文本, nil
}
```
