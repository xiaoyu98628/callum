```go
package 聊天补全

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	HTTP包 "net/http"

	OpenAI包 "github.com/openai/openai-go/v3"
	选项包 "github.com/openai/openai-go/v3/option"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 流 同步发送文本增量，响应关闭成功后再发送完成事件。
func (当前客户端 *客户端) 流式生成(执行上下文 上下文包.上下文, 请求 模型包.请求, 发出事件 func(模型包.事件) error) error {
	if 发出事件 == nil {
		return 错误判断包.新建("流式事件回调不能为空")
	}

	参数, 错误 := 当前客户端.构建请求(执行上下文, 请求)
	if 错误 != nil {
		return 错误
	}

	var 捕获 *流捕获器
	if 请求.捕获结果 || len(请求.工具集) > 0 {
		捕获 = &流捕获器{}
		参数.流选项集 = OpenAI包.对话生成流选项集参数{包含用量: OpenAI包.布尔(true)}
	}
	HTTP响应, 错误 := 当前客户端.流文本(执行上下文, 参数, 发出事件, 捕获)
	if 捕获 != nil {
		结果, 捕获错误 := 捕获.结果(请求标识(HTTP响应))
		if 捕获错误 == nil && 错误 == nil {
			结果.工具调用集, 捕获错误 = 响应调用集(string(捕获.消息JSON()), 结果.结束原因, len(请求.工具集) > 0)
		}
		错误 = 错误判断包.合并(错误, 捕获错误)
		if 回调错误 := 发出事件(模型包.事件{类型: 模型包.事件结果, 请求标识: 结果.请求标识, 结果: &结果}); 回调错误 != nil {
			错误 = 错误判断包.合并(错误, 包装错误("callback", HTTP响应, 回调错误))
		}
	}
	if 错误 != nil {
		return 错误
	}

	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 包装错误("stream", HTTP响应, 错误)
	}

	if 错误 := 发出事件(模型包.事件{
		类型:   模型包.事件已完成,
		请求标识: 请求标识(HTTP响应),
	}); 错误 != nil {
		return 包装错误("callback", HTTP响应, 错误)
	}

	return nil
}

// 流文本 负责关闭响应，返回的响应仅供上层读取状态和请求标识。
func (当前客户端 *客户端) 流文本(
	执行上下文 上下文包.上下文,
	参数 OpenAI包.对话生成新建参数,
	发出事件 func(模型包.事件) error,
	捕获 *流捕获器,
) (HTTP响应 *HTTP包.响应, 返回错误 error) {
	流 := 当前客户端.SDK.新建流式(
		执行上下文,
		参数,
		选项包.带有响应进入(&HTTP响应),
	)

	defer func() {
		if 错误 := 流.关闭(); 错误 != nil {
			返回错误 = 错误判断包.合并(
				返回错误,
				包装错误("close stream", HTTP响应, 错误),
			)
		}
	}()

	var 结束原因 string

	for 流.下一个() {
		if 错误 := 执行上下文.获取错误(); 错误 != nil {
			return HTTP响应, 包装错误("stream", HTTP响应, 错误)
		}

		分片 := 流.当前()
		if 捕获 != nil {
			if 错误 := 捕获.添加(分片.原始JSON()); 错误 != nil {
				return HTTP响应, 包装错误("decode stream result", HTTP响应, 错误)
			}
		}
		var 选项集 []JSON包.原始JSON消息
		if 错误 := JSON包.反序列化([]byte(分片.JSON.选项集.原始()), &选项集); 错误 != nil || 选项集 == nil {
			return HTTP响应, 包装错误("decode stream", HTTP响应, 模型包.无效响应错误)
		}

		// 用量等附加分片的 选项集 可以为空数组。
		if len(选项集) == 0 {
			continue
		}

		if len(选项集) != 1 || len(分片.选项集) != 1 {
			return HTTP响应, 包装错误(
				"decode stream",
				HTTP响应,
				模型包.无效响应错误,
			)
		}

		选项 := 分片.选项集[0]
		if 错误 := 校验选项(选项.原始JSON()); 错误 != nil {
			return HTTP响应, 包装错误("decode stream", HTTP响应, 错误)
		}
		if 错误 := 校验文本输出(选项.增量.原始JSON()); 错误 != nil {
			return HTTP响应, 包装错误("decode stream", HTTP响应, 错误)
		}

		// 已验证的服务会在结束后发送带空 选项 的用量分片。
		// 仅跳过纯用量分片，继续生成内容或再次指定结束原因仍视为异常。
		if 结束原因 != "" {
			if 是尾部用量分片(分片) {
				continue
			}
			return HTTP响应, 包装错误(
				"decode stream",
				HTTP响应,
				模型包.无效响应错误,
			)
		}

		if len(选项.增量.工具调用集) > 0 && len(参数.工具集) == 0 {
			return HTTP响应, 包装错误(
				"decode stream",
				HTTP响应,
				模型包.不支持输出错误,
			)
		}

		if 选项.增量.拒绝 != "" {
			return HTTP响应, 包装错误(
				"decode stream",
				HTTP响应,
				模型包.已拒绝错误,
			)
		}

		推理, 错误 := 推理文本(选项.增量.原始JSON())
		if 错误 != nil {
			return HTTP响应, 包装错误("decode reasoning", HTTP响应, 错误)
		}
		if 推理 != nil {
			if 错误 := 发出事件(模型包.事件{类型: 模型包.事件推理增量, 文本: *推理, 请求标识: 请求标识(HTTP响应)}); 错误 != nil {
				return HTTP响应, 包装错误("callback", HTTP响应, 错误)
			}
		}
		if 选项.增量.内容 != "" {
			if 错误 := 发出事件(模型包.事件{
				类型:   模型包.事件文本增量,
				文本:   选项.增量.内容,
				请求标识: 请求标识(HTTP响应),
			}); 错误 != nil {
				return HTTP响应, 包装错误("callback", HTTP响应, 错误)
			}
		}

		if 选项.结束原因 != "" {
			结束原因 = 选项.结束原因
		}
	}

	if 错误 := 流.获取错误(); 错误 != nil {
		return HTTP响应, 包装错误("read stream", HTTP响应, 错误)
	}

	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return HTTP响应, 包装错误("stream", HTTP响应, 错误)
	}

	// 读取结束不等于生成成功，必须检查模型的结束原因。
	if 错误 := 检查工具结束原因(结束原因, len(参数.工具集) > 0); 错误 != nil {
		return HTTP响应, 包装错误("finish stream", HTTP响应, 错误)
	}

	return HTTP响应, nil
}

func 是尾部用量分片(分片 OpenAI包.对话生成分片) bool {
	if len(分片.选项集) != 1 {
		return false
	}
	选项 := 分片.选项集[0]
	if 原因 := 选项.JSON.结束原因.原始(); 原因 != "" && 原因 != "null" {
		return false
	}

	// 检查原始 增量 必须是空对象，避免漏掉当前未支持的生成字段。
	var 增量 map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化([]byte(选项.增量.原始JSON()), &增量); 错误 != nil || 增量 == nil || len(增量) != 0 {
		return false
	}

	var 用量 map[string]JSON包.原始JSON消息
	return JSON包.反序列化([]byte(分片.JSON.用量.原始()), &用量) == nil && 用量 != nil
}
```
