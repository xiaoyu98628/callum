```go
package 服务

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	输入输出包 "io"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	压缩包 "github.com/xiaoyu98628/callum/internal/conversation/compaction"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

var 摘要超时错误 = 错误判断包.新建("生成历史摘要超时")

// 请求失败 只保留受控错误信息，不保存可能回显凭据的供应商错误体。
type 请求失败 struct {
	HTTP状态 int    `json:"http_status,omitempty"`
	操作     string `json:"operation"`
	请求标识   string `json:"provider_request_id,omitempty"`
}

func 填充失败(结果 *模型包.响应, 协议 string, 错误 error) {
	调用, 成功 := 错误判断包.按类型匹配[*模型包.调用错误](错误)
	if !成功 {
		return
	}
	if 结果.请求标识 == "" {
		结果.请求标识 = 调用.请求标识
	}
	if 结果.输出 != nil {
		return
	}
	信封 := struct {
		协议 string `json:"protocol"`
		格式 string `json:"format"`
		错误 请求失败   `json:"error"`
	}{协议, "request_error", 请求失败{调用.状态代码, 调用.操作, 调用.请求标识}}
	// 结构仅含字符串和整数，JSON 编码不可能失败。
	结果.输出, _ = JSON包.序列化(信封)
}

func 失败(错误 error) (状态, 类别, 消息 string) {
	状态 = 智能体包.失败
	switch {
	case 错误判断包.是否属于(错误, 模型包.词元预算错误):
		return 状态, "token_budget_unconfigured", "请配置模型上下文容量或有效的输入 Token 预算"
	case 错误判断包.是否属于(错误, 模型包.词元上限错误):
		return 状态, "token_budget_exceeded", "保留上下文或单个完整轮次超过 Token 预算，请调整预算或保留轮次"
	case 错误判断包.是否属于(错误, 压缩包.无减少错误):
		return 状态, "compaction_not_reduced", "摘要未能缩小上下文，原有效版本已保留"
	case 错误判断包.是否属于(错误, 智能体包.存储错误):
		return 状态, "internal", "保存执行数据失败"
	case 错误判断包.是否属于(错误, 会话包.已引导错误):
		return 智能体包.已取消, "steered", "已按引导切换模型请求"
	case 错误判断包.是否属于(错误, 上下文包.已取消错误):
		return 智能体包.已取消, "cancelled", "执行已取消"
	case 错误判断包.是否属于(错误, 摘要超时错误):
		return 状态, "timeout", "生成历史摘要超时，原有历史已保留"
	case 错误判断包.是否属于(错误, 上下文包.超出截止时间错误):
		return 状态, "timeout", "执行超时"
	case 错误判断包.是否属于(错误, 智能体包.输出上限错误):
		return 状态, "output_limit", "内容超过字节上限"
	case 错误判断包.是否属于(错误, 模型包.不支持输出错误), 错误判断包.是否属于(错误, 推理调用包.不支持错误):
		return 状态, "unsupported", "当前适配器不支持该输入或输出"
	case 错误判断包.是否属于(错误, 模型包.已拒绝错误):
		return 状态, "refused", "模型拒绝生成内容"
	case 错误判断包.是否属于(错误, 模型包.不完整错误), 错误判断包.是否属于(错误, 输入输出包.非预期文件结束错误):
		return 状态, "incomplete", "模型输出未完整完成"
	case 错误判断包.是否属于(错误, 模型包.无效响应错误):
		if 详情, 成功 := 错误判断包.按类型匹配[*模型包.工具参数错误](错误); 成功 {
			return 状态, "invalid_response", 详情.错误文本()
		}
		if 详情, 成功 := 错误判断包.按类型匹配[*模型包.工具纠正停滞错误](错误); 成功 {
			return 状态, "invalid_response", 详情.错误文本()
		}
		return 状态, "invalid_response", "模型响应格式无效"
	case 错误判断包.是否属于(错误, 模型包.图片上限错误):
		return 状态, "image_limit", 模型包.图片上限错误.错误文本()
	case 错误判断包.是否属于(错误, 模型包.附件错误):
		return 状态, "attachment_unavailable", 模型包.附件错误.错误文本()
	case 错误判断包.是否属于(错误, 模型包.图片读取错误):
		return 状态, "image_read", "图片文件不可读取"
	case 错误判断包.是否属于(错误, 模型包.图片格式错误):
		return 状态, "image_format", "图片格式不支持"
	case 错误判断包.是否属于(错误, 模型包.输入上限错误):
		if 参数, 成功 := 错误判断包.按类型匹配[*模型包.工具参数错误](错误); 成功 {
			return 状态, "tool_arguments_limit", 参数.错误文本()
		}
		if 上限, 成功 := 错误判断包.按类型匹配[*模型包.输入上限错误详情](错误); 成功 {
			return 状态, "input_limit", 上限.错误文本()
		}
		return 状态, "input_limit", "模型输入超过字节上限"
	default:
		if 调用, 成功 := 错误判断包.按类型匹配[*模型包.调用错误](错误); 成功 && 调用.状态代码 > 0 {
			return 状态, "model_failed", 格式化包.格式化字符串("模型调用失败（HTTP %d）", 调用.状态代码)
		}
		return 状态, "model_failed", "执行失败"
	}
}

// 操作存储错误 区分受理或运行上下文取消、内容边界与真正的持久化故障。
// 终态清理写入失败仍由调用方显式标记 存储错误，不能当作普通取消忽略。
func 操作存储错误(执行上下文 上下文包.上下文, 错误 error) error {
	if 错误 == nil {
		return nil
	}
	if 执行上下文.获取错误() != nil {
		return 错误判断包.合并(执行上下文.获取错误(), 错误)
	}
	if 错误判断包.是否属于(错误, 模型包.输入上限错误) || 错误判断包.是否属于(错误, 智能体包.输出上限错误) {
		return 错误
	}
	return 错误判断包.合并(智能体包.存储错误, 错误)
}
```
