```go
package 聊天补全

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	错误判断包 "errors"
	格式化包 "fmt"
	HTTP包 "net/http"
	字符串包 "strings"
	测试包 "testing"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test工具定义集并且消息集(测试器 *测试包.测试器) {
	请求 := 模型包.请求{模型: "m", 工具集: []模型包.工具定义{{名称: "read_file", 说明: "读取", 参数: JSON包.原始JSON消息(`{"type":"object"}`)}}, 消息集: []模型包.消息{{角色: 模型包.角色用户, 内容: "读取"}, {角色: 模型包.角色助手, 工具调用集: []模型包.工具调用{{标识: "c", 名称: "read_file", 参数: JSON包.原始JSON消息(`{"path":"a"}`)}}}, {角色: 模型包.角色工具, 工具调用标识: "c", 内容: `{"text":"ok"}`}}}
	参数, 错误 := (&客户端{}).构建请求(测试器.上下文(), 请求)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	原始, 错误 := JSON包.序列化(参数)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 数据 struct {
		工具集 []JSON包.原始JSON消息            `json:"tools"`
		消息集 []map[string]JSON包.原始JSON消息 `json:"messages"`
		选项  any                         `json:"tool_choice"`
	}
	if 错误 := JSON包.反序列化(原始, &数据); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if len(数据.工具集) != 1 || len(数据.消息集) != 3 || string(数据.消息集[2]["tool_call_id"]) != `"c"` || 数据.选项 != nil {
		测试器.格式化报告致命错误("请求未保持自动选择和调用配对: %s", 原始)
	}
}

func Test工具定义集拥有无本地数量上限(测试器 *测试包.测试器) {
	请求 := 测试请求()
	for 索引 := range 64 {
		请求.工具集 = append(请求.工具集, 模型包.工具定义{名称: 格式化包.格式化字符串("tool_%d", 索引), 参数: JSON包.原始JSON消息(`{"type":"object"}`)})
	}
	参数, 错误 := (&客户端{}).构建请求(测试器.上下文(), 请求)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	原始, 错误 := JSON包.序列化(参数)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 已发送 struct {
		工具集 []JSON包.原始JSON消息 `json:"tools"`
	}
	if 错误 := JSON包.反序列化(原始, &已发送); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if len(已发送.工具集) != 64 {
		测试器.格式化报告致命错误("工具定义被拒绝或截断: %d", len(已发送.工具集))
	}
	// 去掉数量阈值仍须拒绝重复定义及非法名称。
	请求.工具集 = append(请求.工具集, 请求.工具集[0])
	if _, 错误 := (&客户端{}).构建请求(测试器.上下文(), 请求); !错误判断包.是否属于(错误, 模型包.无效响应错误) {
		测试器.格式化报告致命错误("接受重复工具: %v", 错误)
	}
}

func Test大型工具调用批次用于生成并且流(测试器 *测试包.测试器) {
	for _, 流 := range []bool{false, true} {
		测试器.执行(格式化包.转为字符串(流), func(测试器 *测试包.测试器) {
			调用集 := make([]map[string]any, 40)
			for 索引 := range 调用集 {
				调用集[索引] = map[string]any{"id": 格式化包.格式化字符串("call-%d", 索引), "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}}
				if 流 {
					调用集[索引]["index"] = 索引
				}
			}
			选项 := map[string]any{"index": 0, "finish_reason": "tool_calls"}
			if 流 {
				选项["delta"] = map[string]any{"tool_calls": 调用集}
			} else {
				选项["message"] = map[string]any{"role": "assistant", "tool_calls": 调用集}
			}
			载荷, 错误 := JSON包.序列化(map[string]any{"choices": []any{选项}})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			通信 := string(载荷)
			if 流 {
				通信 = SSE(通信, "[DONE]")
			}
			正文 := &已跟踪正文{读取器: 字符串包.新建读取器(通信)}
			客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
				内容类型 := "application/json"
				if 流 {
					内容类型 = "text/event-stream"
				}
				return 测试响应(请求, 内容类型, 正文), nil
			})
			请求 := 测试请求()
			请求.工具集 = []模型包.工具定义{{名称: "read_file", 参数: []byte(`{"type":"object"}`)}}
			var 响应 模型包.响应
			if 流 {
				错误 = 客户端.流式生成(测试器.上下文(), 请求, func(事件 模型包.事件) error {
					if 事件.结果 != nil {
						响应 = *事件.结果
					}
					return nil
				})
			} else {
				响应, 错误 = 客户端.生成(测试器.上下文(), 请求)
			}
			if 错误 != nil || len(响应.工具调用集) != 40 || 正文.关闭次数 != 1 {
				测试器.格式化报告致命错误("大批调用被截断或资源未释放: calls=%d closes=%d err=%v", len(响应.工具调用集), 正文.关闭次数, 错误)
			}
			for 索引, 调用 := range 响应.工具调用集 {
				if 调用.标识 != 格式化包.格式化字符串("call-%d", 索引) || string(调用.参数) != `{"path":"a.txt"}` {
					测试器.报告致命错误("调用顺序或参数丢失")
				}
			}
		})
	}
}

func Test工具调用字节并且结构边界集保留(测试器 *测试包.测试器) {
	var 捕获 工具调用捕获
	// 单个 索引 不决定调用配额，但缺口、负值、重复 标识 和过大的参数仍应拒绝。
	if 错误 := 捕获.添加([]byte(`[{"index":40,"id":"sparse","type":"function","function":{"name":"read_file","arguments":"{}"}}]`)); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 捕获.结果(); !错误判断包.是否属于(错误, 模型包.无效响应错误) {
		测试器.报告致命错误("接受缺口索引")
	}
	if 错误 := 捕获.添加([]byte(`[{"index":-1}]`)); !错误判断包.是否属于(错误, 模型包.无效响应错误) {
		测试器.报告致命错误("接受负索引")
	}
	参数 := `{"content":"` + 字符串包.重复("x", 最大工具参数) + `"}`
	调用 := map[string]any{"id": "large", "type": "function", "function": map[string]string{"name": "read_file", "arguments": 参数}}
	原始, 错误 := JSON包.序列化(map[string]any{"tool_calls": []any{调用}})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 响应调用集(string(原始), "tool_calls", true); 错误 == nil {
		测试器.报告致命错误("取消数量限制时放开了参数字节边界")
	}
	// 累计字段字节边界对完整响应与流式分片一致，单项参数均在合法范围内。
	调用["function"] = map[string]string{"name": "read_file", "arguments": `{"content":"` + 字符串包.重复("x", 16000) + `"}`}
	调用集 := make([]any, 525)
	for 索引 := range 调用集 {
		调用集[索引] = map[string]any{"id": 格式化包.格式化字符串("c-%d", 索引), "type": 调用["type"], "function": 调用["function"]}
	}
	原始, 错误 = JSON包.序列化(map[string]any{"tool_calls": 调用集})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 响应调用集(string(原始), "tool_calls", true); !错误判断包.是否属于(错误, 模型包.输入上限错误) {
		测试器.格式化报告致命错误("完整调用总字节边界失效: %v", 错误)
	}
	var 片段集 工具调用捕获
	for 索引 := range 调用集 {
		调用集[索引].(map[string]any)["index"] = 索引
		原始, 错误 := JSON包.序列化([]any{调用集[索引]})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		错误 = 片段集.添加(原始)
		if 错误判断包.是否属于(错误, 模型包.输入上限错误) {
			return
		}
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	测试器.报告致命错误("流式调用总字节边界失效")
}

func Test超限工具参数拥有可恢复详情(测试器 *测试包.测试器) {
	参数 := `{"patch":"` + 字符串包.重复("中", 5458) + `"}` // UTF-8 共 16,386 字节。
	调用 := map[string]any{"index": 0, "id": "large", "type": "function", "function": map[string]string{"name": "apply_patch", "arguments": 参数}}
	完整, 错误 := JSON包.序列化(map[string]any{"tool_calls": []any{调用}})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	_, 完成错误 := 响应调用集(string(完整), "tool_calls", true)
	原始, 错误 := JSON包.序列化([]any{调用})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 捕获 工具调用捕获
	流错误 := 捕获.添加(原始)
	for _, 失败 := range []error{完成错误, 流错误} {
		详情, 成功 := 错误判断包.按类型匹配[*模型包.工具参数错误](失败)
		if !成功 || !错误判断包.是否属于(失败, 模型包.输入上限错误) || 详情.工具 != "apply_patch" || 详情.实际字节 != len(参数) || 详情.上限字节 != 最大工具参数 || 详情.索引 != 0 || 详情.指纹 == "" {
			测试器.格式化报告致命错误("参数超限详情丢失: %v", 失败)
		}
	}
	if 完成错误.(*模型包.工具参数错误).指纹 != 流错误.(*模型包.工具参数错误).指纹 {
		测试器.报告致命错误("相同参数的流式与完整响应指纹不同")
	}
}

func Test工具流组装并且校验(测试器 *测试包.测试器) {
	var 捕获 流捕获器
	分片集 := []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c","type":"function","vendor":"call-meta","function":{"vendor":{"trace":"kept"},"name":"read_","arguments":"{\"path\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"file","arguments":"\"a\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"total_tokens":9,"vendor":"kept"}}`,
	}
	for _, 分片 := range 分片集 {
		if 错误 := 捕获.添加(分片); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	结果, 错误 := 捕获.结果("request")
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if !字节包.包含(结果.输出, []byte("call-meta")) || !字节包.包含(结果.输出, []byte("kept")) {
		测试器.格式化报告致命错误("工具扩展字段丢失: %s", 结果.输出)
	}
	调用集, 错误 := 响应调用集(string(捕获.消息JSON()), 结果.结束原因, true)
	if 错误 != nil || len(调用集) != 1 || 调用集[0].名称 != "read_file" || string(调用集[0].参数) != `{"path":"a"}` || string(结果.用量) != `{"total_tokens":9,"vendor":"kept"}` {
		测试器.格式化报告致命错误("工具组装失败: %+v %s %v", 调用集, 结果.用量, 错误)
	}
	for _, 原始 := range []string{`{"tool_calls":[{"id":"c","type":"function","function":{"name":"read_file","arguments":"{"}}]}`, `{"tool_calls":[{"id":"c","type":"function","function":{"name":"read_file","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"read_file","arguments":"{}"}}]}`} {
		if _, 错误 := 响应调用集(原始, "tool_calls", true); !错误判断包.是否属于(错误, 模型包.无效响应错误) {
			测试器.格式化报告致命错误("接受无效或重复调用: %v", 错误)
		}
	}
	if _, 错误 := 响应调用集(string(捕获.消息JSON()), "tool_calls", false); !错误判断包.是否属于(错误, 模型包.不支持输出错误) {
		测试器.报告致命错误(错误)
	}
}

func Test无效工具JSON保留整体响应无调用集(测试器 *测试包.测试器) {
	for _, 流 := range []bool{false, true} {
		测试器.执行(格式化包.转为字符串(流), func(测试器 *测试包.测试器) {
			无效 := `{"["string""]</｜｜DSML｜｜: `
			调用集 := []map[string]any{
				{"id": "valid", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}},
				{"id": "invalid", "type": "function", "function": map[string]string{"name": "read_file", "arguments": 无效}},
			}
			消息 := map[string]any{"role": "assistant", "content": "准备读取", "tool_calls": 调用集}
			选项 := map[string]any{"index": 0, "finish_reason": "tool_calls"}
			if 流 {
				for 索引 := range 调用集 {
					调用集[索引]["index"] = 索引
				}
				选项["delta"] = 消息
			} else {
				选项["message"] = 消息
			}
			已编码, _ := JSON包.序列化(map[string]any{"choices": []any{选项}, "usage": map[string]int{"prompt_tokens": 13, "total_tokens": 17}})
			通信 := string(已编码)
			if 流 {
				通信 = SSE(通信, "[DONE]")
			}
			正文 := &已跟踪正文{读取器: 字符串包.新建读取器(通信)}
			客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
				类别 := "application/json"
				if 流 {
					类别 = "text/event-stream"
				}
				return 测试响应(请求, 类别, 正文), nil
			})
			请求 := 测试请求()
			请求.工具集 = []模型包.工具定义{{名称: "read_file", 参数: []byte(`{"type":"object"}`)}}
			var 响应 模型包.响应
			var 错误 error
			已完成 := false
			if 流 {
				错误 = 客户端.流式生成(测试器.上下文(), 请求, func(事件 模型包.事件) error {
					if 事件.结果 != nil {
						响应 = *事件.结果
					}
					if 事件.类型 == 模型包.事件已完成 {
						已完成 = true
					}
					return nil
				})
			} else {
				响应, 错误 = 客户端.生成(测试器.上下文(), 请求)
			}
			详情, 成功 := 错误判断包.按类型匹配[*模型包.工具参数错误](错误)
			if !成功 || 详情.索引 != 1 || 详情.指纹 == "" || len(响应.工具调用集) != 0 || 响应.结束原因 != "tool_calls" || !字节包.包含(响应.输出, []byte("DSML")) || 响应.输入词元 == nil || *响应.输入词元 != 13 || 正文.关闭次数 != 1 || 已完成 {
				测试器.格式化报告致命错误("坏参数未完整保留或被当作成功: response=%+v close=%d err=%v", 响应, 正文.关闭次数, 错误)
			}
		})
	}
	// 身份错误与资源越界优先，不能借坏参数把损坏协议转换成可重试错误。
	for _, 原始 := range []string{
		`{"tool_calls":[{"id":"same","type":"function","function":{"name":"read_file","arguments":"{"}},{"id":"same","type":"function","function":{"name":"read_file","arguments":"{}"}}]}`,
		`{"tool_calls":[{"id":"bad","type":"function","function":{"name":"invalid name","arguments":"{"}}]}`,
	} {
		_, 错误 := 响应调用集(原始, "tool_calls", true)
		if _, 成功 := 错误判断包.按类型匹配[*模型包.工具参数错误](错误); 成功 || !错误判断包.是否属于(错误, 模型包.无效响应错误) {
			测试器.格式化报告致命错误("错误的工具身份被当作可纠正参数: %v", 错误)
		}
	}
}
```
