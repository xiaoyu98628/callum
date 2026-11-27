```go
package 聊天补全

import (
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	HTTP包 "net/http"
	切片包 "slices"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"

	OpenAI包 "github.com/openai/openai-go/v3"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

const 测试请求标识 = "req-test"

type 轮次往返函数 func(*HTTP包.请求) (*HTTP包.响应, error)

func (当前轮次往返函数 轮次往返函数) 轮次往返(请求 *HTTP包.请求) (响应 *HTTP包.响应, 错误 error) {
	defer func() {
		if 请求.正文 != nil {
			错误 = 错误判断包.合并(错误, 请求.正文.关闭())
		}
	}()
	return 当前轮次往返函数(请求)
}

type 已跟踪正文 struct {
	输入输出包.读取器
	关闭次数   int
	关闭错误 error
}

func (当前已跟踪正文 *已跟踪正文) 关闭() error {
	当前已跟踪正文.关闭次数++
	return 当前已跟踪正文.关闭错误
}

func 新建测试客户端(测试器 *测试包.测试器, 传输模块 轮次往返函数) *客户端 {
	测试器.标记测试辅助函数()
	客户端, 错误 := 新建(配置{
		基础URL: "https://model.invalid/v1",
		API密钥:  "test-api-key",
	}, &HTTP包.客户端{传输: 传输模块})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 客户端
}

func 测试响应(请求 *HTTP包.请求, 内容类型 string, 正文 输入输出包.读取关闭器) *HTTP包.响应 {
	return &HTTP包.响应{
		状态代码: HTTP包.状态成功,
		头部: HTTP包.头部{
			"Content-Type": {内容类型},
			"X-Request-Id": {测试请求标识},
		},
		正文:    正文,
		请求: 请求,
	}
}

func 测试请求() 模型包.请求 {
	return 模型包.请求{
		模型: "remote-model",
		消息集: []模型包.消息{
			{角色: 模型包.角色用户, 内容: "你好"},
		},
	}
}

func 断言调用错误(测试器 *测试包.测试器, 错误 error, 状态代码 int, 请求标识 string) *模型包.调用错误 {
	测试器.标记测试辅助函数()
	调用错误, 成功 := 错误判断包.按类型匹配[*模型包.调用错误](错误)
	if !成功 {
		测试器.格式化报告致命错误("错误类型为 %T，期望 *model.CallError", 错误)
	}
	if 调用错误.状态代码 != 状态代码 || 调用错误.请求标识 != 请求标识 {
		测试器.格式化创建错误("错误元信息为 status=%d, request_id=%q，期望 status=%d, request_id=%q",
			调用错误.状态代码, 调用错误.请求标识, 状态代码, 请求标识)
	}
	return 调用错误
}

func Test生成转换消息集并且释放响应(测试器 *测试包.测试器) {
	正文 := &已跟踪正文{读取器: 字符串包.新建读取器(`{"choices":[{"index":0,"message":{"role":"assistant","content":"你好"},"finish_reason":"stop"}]}`)}
	输入 := 模型包.请求{
		模型: "remote-model",
		消息集: []模型包.消息{
			{角色: 模型包.角色系统, 内容: "用中文回答"},
			{角色: 模型包.角色用户, 内容: "你好"},
			{角色: 模型包.角色助手, 内容: "你好"},
		},
	}
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		if 请求.方法 != HTTP包.方法提交 || 请求.URL.路径 != "/v1/chat/completions" {
			测试器.格式化创建错误("请求为 %s %s", 请求.方法, 请求.URL.路径)
		}
		if 请求.头部.获取("Authorization") != "Bearer test-api-key" {
			测试器.错误文本("认证头未使用传入的测试凭据")
		}
		var 通信 struct {
			模型    string          `json:"model"`
			消息集 []模型包.消息 `json:"messages"`
			流   bool            `json:"stream"`
		}
		if 错误 := JSON包.新建解码器(请求.正文).解码(&通信); 错误 != nil {
			return nil, 错误
		}
		if 通信.模型 != 输入.模型 || !切片包.相等函数(通信.消息集, 输入.消息集, func(当前消息, 当前消息 模型包.消息) bool { return 当前消息.角色 == 当前消息.角色 && 当前消息.内容 == 当前消息.内容 }) || 通信.流 {
			测试器.格式化创建错误("请求转换错误：%+v", 通信)
		}
		return 测试响应(请求, "application/json", 正文), nil
	})

	结果, 错误 := 客户端.生成(测试器.上下文(), 输入)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 结果.文本 != "你好" || 结果.请求标识 != 测试请求标识 {
		测试器.格式化创建错误("响应为 %+v", 结果)
	}
	if 正文.关闭次数 != 1 {
		测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
	}
}

func Test生成拒绝无效响应集(测试器 *测试包.测试器) {
	for _, 载荷 := range []string{
		"null",
		`{"choices":[]}`,
		`{"choices":[{"index":1}]}`,
		`{"choices":[{"index":0,"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":null,"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":[],"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":null},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":123},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":[]},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":"ok","refusal":123},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":"ok","tool_calls":{}},"finish_reason":"stop"}]}`,
		`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":"0","message":{"content":"ok"},"finish_reason":"stop"}]}`,
		`{"choices":[{"index":0,"message":{"content":"ok"},"finish_reason":123}]}`,
	} {
		测试器.执行(载荷, func(测试器 *测试包.测试器) {
			正文 := &已跟踪正文{读取器: 字符串包.新建读取器(载荷)}
			客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
				return 测试响应(请求, "application/json", 正文), nil
			})
			defer func() {
				if 已恢复 := recover(); 已恢复 != nil {
					测试器.格式化创建错误("异常响应触发 panic：%v", 已恢复)
				}
				if 正文.关闭次数 != 1 {
					测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
				}
			}()

			_, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
			if !错误判断包.是否属于(错误, 模型包.无效响应错误) {
				测试器.格式化报告致命错误("错误为 %v，期望 ErrInvalidResponse", 错误)
			}
			断言调用错误(测试器, 错误, HTTP包.状态成功, 测试请求标识)
		})
	}
}

func Test生成保留部分文本(测试器 *测试包.测试器) {
	正文 := &已跟踪正文{读取器: 字符串包.新建读取器(`{"choices":[{"index":0,"message":{"content":"尚未完成"},"finish_reason":"length"}]}`)}
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		return 测试响应(请求, "application/json", 正文), nil
	})
	结果, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
	if !错误判断包.是否属于(错误, 模型包.不完整错误) {
		测试器.格式化报告致命错误("错误为 %v，期望 ErrIncomplete", 错误)
	}
	if 结果.文本 != "尚未完成" || 结果.请求标识 != 测试请求标识 {
		测试器.格式化创建错误("部分响应丢失：%+v", 结果)
	}
	if 正文.关闭次数 != 1 {
		测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
	}
}

func Test生成拒绝工具调用集(测试器 *测试包.测试器) {
	正文 := &已跟踪正文{读取器: 字符串包.新建读取器(`{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call-test","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"stop"}]}`)}
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		return 测试响应(请求, "application/json", 正文), nil
	})
	_, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
	if !错误判断包.是否属于(错误, 模型包.不支持输出错误) {
		测试器.格式化报告致命错误("工具输出被当作文本响应：%v", 错误)
	}
	断言调用错误(测试器, 错误, HTTP包.状态成功, 测试请求标识)
	if 正文.关闭次数 != 1 {
		测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
	}
}

func Test生成分类音频并且扩展集(测试器 *测试包.测试器) {
	for _, 测试 := range []struct {
		名称, 消息 string
		预期错误       error
	}{
		{"audio_only", `{"role":"assistant","content":"","audio":{"id":"audio-1","data":"ZmFrZQ=="}}`, 模型包.不支持输出错误},
		{"audio_with_text", `{"role":"assistant","content":"正文","audio":{"id":"audio-1"}}`, 模型包.不支持输出错误},
		{"empty_audio_object", `{"content":"","audio":{}}`, 模型包.不支持输出错误},
		{"null_audio", `{"content":"正文","audio":null}`, nil},
		{"provider_metadata", `{"content":"正文","vendor_meta":{"trace":"kept"}}`, nil},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			载荷 := `{"choices":[{"index":0,"message":` + 测试.消息 + `,"finish_reason":"stop"}],"usage":{"total_tokens":7,"vendor":"kept"}}`
			正文 := &已跟踪正文{读取器: 字符串包.新建读取器(载荷)}
			客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
				return 测试响应(请求, "application/json", 正文), nil
			})
			结果, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
			if !错误判断包.是否属于(错误, 测试.预期错误) {
				测试器.格式化报告致命错误("输出分类错误: %v，期望 %v", 错误, 测试.预期错误)
			}
			if 测试.预期错误 != nil {
				断言调用错误(测试器, 错误, HTTP包.状态成功, 测试请求标识)
			}
			if !字符串包.包含(string(结果.输出), 测试.消息) || !字符串包.包含(string(结果.用量), `"vendor":"kept"`) || 结果.请求标识 != 测试请求标识 {
				测试器.格式化报告致命错误("原始输出或实际 usage 丢失: %+v", 结果)
			}
			if 正文.关闭次数 != 1 {
				测试器.格式化报告致命错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
			}
		})
	}
}

func Test生成传递调用方上下文(测试器 *测试包.测试器) {
	type 上下文键 struct{}
	执行上下文, 取消函数 := 上下文包.创建可取消上下文(上下文包.带有值(测试器.上下文(), 上下文键{}, "caller"))
	defer 取消函数()
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		if 请求.上下文().值(上下文键{}) != "caller" {
			测试器.错误文本("调用方上下文丢失")
		}
		取消函数()
		return nil, 请求.上下文().获取错误()
	})
	_, 错误 := 客户端.生成(执行上下文, 测试请求())
	if !错误判断包.是否属于(错误, 上下文包.已取消错误) {
		测试器.格式化报告致命错误("取消错误链丢失：%v", 错误)
	}
}

func Test生成遵循已过期截止时间(测试器 *测试包.测试器) {
	执行上下文, 取消函数 := 上下文包.创建截止时间上下文(测试器.上下文(), 时间包.当前时间().添加(-时间包.秒))
	defer 取消函数()
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		return nil, 请求.上下文().获取错误()
	})
	_, 错误 := 客户端.生成(执行上下文, 测试请求())
	if !错误判断包.是否属于(错误, 上下文包.超出截止时间错误) {
		测试器.格式化报告致命错误("超时错误链丢失：%v", 错误)
	}
}

func Test新建忽略环境选项集(测试器 *测试包.测试器) {
	测试器.设置环境变量("OPENAI_BASE_URL", "https://environment.invalid/v1")
	测试器.设置环境变量("OPENAI_API_KEY", "environment-api-key")
	测试器.设置环境变量("OPENAI_ADMIN_KEY", "environment-admin-key")
	测试器.设置环境变量("OPENAI_ORG_ID", "environment-org")
	测试器.设置环境变量("OPENAI_PROJECT_ID", "environment-project")
	测试器.设置环境变量("OPENAI_CUSTOM_HEADERS", "X-Environment: environment-value\nAuthorization: Bearer environment-key")

	for _, API键 := range []string{"test-api-key", ""} {
		测试器.执行(API键, func(测试器 *测试包.测试器) {
			客户端, 错误 := 新建(配置{基础URL: "https://model.invalid/v1", API密钥: API键}, &HTTP包.客户端{
				传输: 轮次往返函数(func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
					if 请求.URL.主机 != "model.invalid" || 请求.URL.路径 != "/v1/chat/completions" {
						测试器.错误文本("请求地址未使用显式配置")
					}
					预期认证 := ""
					if API键 != "" {
						预期认证 = "Bearer " + API键
					}
					if 请求.头部.获取("Authorization") != 预期认证 {
						测试器.错误文本("认证头未使用显式配置")
					}
					for _, 头部 := range []string{"OpenAI-Organization", "OpenAI-Project", "X-Environment"} {
						if 请求.头部.获取(头部) != "" {
							测试器.格式化创建错误("请求继承了环境请求头 %s", 头部)
						}
					}
					var 通信 struct {
						流 bool `json:"stream"`
					}
					if 错误 := JSON包.新建解码器(请求.正文).解码(&通信); 错误 != nil {
						return nil, 错误
					}
					if 通信.流 {
						return 测试响应(请求, "text/event-stream", &已跟踪正文{
							读取器: 字符串包.新建读取器(SSE(文本分片, 停止分片, "[DONE]")),
						}), nil
					}
					return 测试响应(请求, "application/json", &已跟踪正文{
						读取器: 字符串包.新建读取器(`{"choices":[{"index":0,"message":{"content":"ok"},"finish_reason":"stop"}]}`),
					}), nil
				}),
			})
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if _, 错误 := 客户端.生成(测试器.上下文(), 测试请求()); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 客户端.流式生成(测试器.上下文(), 测试请求(), func(模型包.事件) error { return nil }); 错误 != nil {
				测试器.报告致命错误(错误)
			}
		})
	}
}

func Test生成保留输出分类(测试器 *测试包.测试器) {
	测试集 := []struct {
		名称    string
		载荷 string
		预期错误 error
	}{
		{名称: "空文本", 载荷: `{"choices":[{"index":0,"message":{"content":""},"finish_reason":"stop"}]}`},
		{名称: "拒绝输出", 载荷: `{"choices":[{"index":0,"message":{"content":null,"refusal":"无法回答"},"finish_reason":"stop"}]}`, 预期错误: 模型包.已拒绝错误},
		{名称: "工具结束标识", 载荷: `{"choices":[{"index":0,"message":{"content":null},"finish_reason":"tool_calls"}]}`, 预期错误: 模型包.不支持输出错误},
		{名称: "长度截断", 载荷: `{"choices":[{"index":0,"message":{"content":null},"finish_reason":"length"}]}`, 预期错误: 模型包.不完整错误},
		{名称: "缺失结束标识", 载荷: `{"choices":[{"index":0,"message":{"content":"ok"}}]}`, 预期错误: 输入输出包.非预期文件结束错误},
	}
	for _, 测试用例 := range 测试集 {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			正文 := &已跟踪正文{读取器: 字符串包.新建读取器(测试用例.载荷)}
			客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
				return 测试响应(请求, "application/json", 正文), nil
			})
			_, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
			if !错误判断包.是否属于(错误, 测试用例.预期错误) {
				测试器.格式化报告致命错误("错误为 %v，期望 %v", 错误, 测试用例.预期错误)
			}
			if 正文.关闭次数 != 1 {
				测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
			}
		})
	}
}

func Test生成保留关闭错误判断(测试器 *测试包.测试器) {
	读取错误 := 错误判断包.新建("测试读取失败")
	关闭错误 := 错误判断包.新建("测试关闭失败")
	测试集 := []struct {
		名称       string
		读取器     输入输出包.读取器
		状态代码 int
		预期错误    error
		预期文本   string
		预期API错误 bool
	}{
		{名称: "正常响应", 读取器: 字符串包.新建读取器(`{"choices":[{"index":0,"message":{"content":"ok"},"finish_reason":"stop"}]}`), 状态代码: HTTP包.状态成功, 预期文本: "ok"},
		{名称: "无效响应", 读取器: 字符串包.新建读取器(`{"choices":[]}`), 状态代码: HTTP包.状态成功, 预期错误: 模型包.无效响应错误},
		{名称: "读取失败", 读取器: 错误读取器{错误: 读取错误}, 状态代码: HTTP包.状态成功, 预期错误: 读取错误},
		{名称: "HTTP错误", 读取器: 字符串包.新建读取器(`{"error":{"message":"测试服务异常","type":"server_error"}}`), 状态代码: HTTP包.状态无效网关, 预期API错误: true},
	}
	for _, 测试用例 := range 测试集 {
		测试器.执行(测试用例.名称, func(测试器 *测试包.测试器) {
			正文 := &已跟踪正文{读取器: 测试用例.读取器, 关闭错误: 关闭错误}
			客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
				响应 := 测试响应(请求, "application/json", 正文)
				响应.状态代码 = 测试用例.状态代码
				return 响应, nil
			})
			结果, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
			if !错误判断包.是否属于(错误, 关闭错误) || (测试用例.预期错误 != nil && !错误判断包.是否属于(错误, 测试用例.预期错误)) {
				测试器.格式化报告致命错误("原始或关闭错误丢失：%v", 错误)
			}
			if 测试用例.预期API错误 {
				if _, 成功 := 错误判断包.按类型匹配[*OpenAI包.错误](错误); !成功 {
					测试器.报告致命错误("供应商错误链丢失")
				}
			}
			断言调用错误(测试器, 错误, 测试用例.状态代码, 测试请求标识)
			if 结果.文本 != 测试用例.预期文本 {
				测试器.格式化创建错误("响应文本为 %q，期望 %q", 结果.文本, 测试用例.预期文本)
			}
			if 正文.关闭次数 != 1 {
				测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
			}
		})
	}
}

func Test生成重定向失败不关闭响应两次(测试器 *测试包.测试器) {
	重定向错误 := 错误判断包.新建("测试拒绝重定向")
	正文 := &已跟踪正文{读取器: 字符串包.新建读取器("")}
	客户端, 错误 := 新建(配置{基础URL: "https://model.invalid/v1", API密钥: "test-api-key"}, &HTTP包.客户端{
		检查重定向: func(*HTTP包.请求, []*HTTP包.请求) error { return 重定向错误 },
		传输: 轮次往返函数(func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
			响应 := 测试响应(请求, "application/json", 正文)
			响应.状态代码 = HTTP包.状态已找到
			响应.头部.设置("Location", "/redirected")
			return 响应, nil
		}),
	})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if _, 错误 := 客户端.生成(测试器.上下文(), 测试请求()); !错误判断包.是否属于(错误, 重定向错误) {
		测试器.格式化报告致命错误("重定向错误链丢失：%v", 错误)
	}
	if 正文.关闭次数 != 1 {
		测试器.格式化创建错误("响应关闭次数为 %d，期望 1", 正文.关闭次数)
	}
}
```
