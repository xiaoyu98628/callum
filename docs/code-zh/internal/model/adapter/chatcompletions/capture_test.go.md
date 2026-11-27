```go
package 聊天补全

import (
	字节包 "bytes"
	上下文包 "context"
	基准64包 "encoding/base64"
	JSON包 "encoding/json"
	错误判断包 "errors"
	图片包 "image"
	PNG包 "image/png"
	HTTP包 "net/http"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test捕获原始用量并且供应商字段集(测试器 *测试包.测试器) {
	原始 := `{"id":"response-1","provider_extra":{"trace":"kept"},"usage":{"prompt_tokens":4,"completion_tokens":2,"vendor_detail":{"cached":1}},"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`
	正文 := &已跟踪正文{读取器: 字符串包.新建读取器(原始)}
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		return 测试响应(请求, "application/json", 正文), nil
	})
	结果, 错误 := 客户端.生成(测试器.上下文(), 测试请求())
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if !字节包.包含(结果.输出, []byte(`"provider_extra":{"trace":"kept"}`)) || !字节包.包含(结果.用量, []byte(`"vendor_detail":{"cached":1}`)) || 字节包.包含(结果.输出, []byte("test-api-key")) {
		测试器.格式化报告致命错误("原始结果或 usage 未保留: %s %s", 结果.输出, 结果.用量)
	}
	if 正文.关闭次数 != 1 {
		测试器.报告致命错误("响应未释放")
	}
}

func Test输入词元仅使用实际提示词词元(测试器 *测试包.测试器) {
	for _, 原始 := range []string{`{}`, `{"total_tokens":7}`, `{"prompt_tokens":null}`, `{"prompt_tokens":-1}`, `{"prompt_tokens":1.5}`, `{"prompt_tokens":"100"}`} {
		if 实际值 := 输入词元([]byte(原始)); 实际值 != nil {
			测试器.格式化报告致命错误("推测或接受了无效输入用量: %s %d", 原始, *实际值)
		}
	}
	if 实际值 := 输入词元([]byte(`{"prompt_tokens":12,"total_tokens":999}`)); 实际值 == nil || *实际值 != 12 {
		测试器.报告致命错误("实际 prompt_tokens 丢失或使用了累计值")
	}
}

func Test捕获流生成并且中断(测试器 *测试包.测试器) {
	for _, 完成 := range []bool{true, false} {
		测试器.执行(map[bool]string{true: "正常", false: "异常中断"}[完成], func(测试器 *测试包.测试器) {
			分片集 := []string{`{"id":"r1","vendor_meta":"kept","choices":[{"index":0,"delta":{"role":"assistant","content":"你","reasoning_content":"a","vendor_part":"x"},"logprobs":{"content":[{"token":"你"}]},"finish_reason":null}]}`, `{"id":"r1","choices":[{"index":0,"delta":{"content":"好","reasoning_content":"b","vendor_part":"y"},"logprobs":{"content":[{"token":"好"}]},"finish_reason":null}]}`}
			if 完成 {
				分片集 = append(分片集, 字符串包.替换(停止分片, `"index":0,`, `"index":0,"logprobs":null,`, 1), `{"choices":[],"usage":{"total_tokens":7,"vendor":{"value":9}}}`, "[DONE]")
			}
			正文 := &已跟踪正文{读取器: 字符串包.新建读取器(SSE(分片集...))}
			客户端 := 新建流客户端(测试器, 正文)
			请求 := 测试请求()
			请求.捕获结果 = true
			var 结果 *模型包.响应
			已完成 := false
			var 推理 字符串包.构建器
			错误 := 客户端.流式生成(测试器.上下文(), 请求, func(事件 模型包.事件) error {
				if 事件.类型 == 模型包.事件推理增量 {
					推理.写入字符串(事件.文本)
				}
				if 事件.类型 == 模型包.事件结果 {
					if 正文.关闭次数 != 1 {
						测试器.错误文本("结果交付前响应未关闭")
					}
					结果 = 事件.结果
				}
				已完成 = 已完成 || 事件.类型 == 模型包.事件已完成
				return nil
			})
			if (错误 == nil) != 完成 || 已完成 != 完成 || 结果 == nil || 结果.文本 != "你好" {
				测试器.格式化报告致命错误("流式结果不正确: %v %+v", 错误, 结果)
			}
			if 推理.字符串() != "ab" || 结果.推理 == nil || *结果.推理 != "ab" {
				测试器.格式化报告致命错误("统一推理字段或增量丢失: %+v", 结果)
			}
			if !字节包.包含(结果.输出, []byte(`"reasoning_content":"ab"`)) || !字节包.包含(结果.输出, []byte(`"vendor_meta":"kept"`)) {
				测试器.格式化报告致命错误("组装丢失返回字段: %s", 结果.输出)
			}
			if !字节包.包含(结果.输出, []byte(`"vendor_part":["x","y"]`)) || !字节包.包含(结果.输出, []byte(`"content":[{"token":"你"},{"token":"好"}]`)) {
				测试器.格式化报告致命错误("扩展片段或 logprobs 丢失: %s", 结果.输出)
			}
			if 完成 {
				if !字节包.包含(结果.用量, []byte(`"vendor":{"value":9}`)) {
					测试器.格式化报告致命错误("usage 被改写: %s", 结果.用量)
				}
			} else if 结果.用量 != nil {
				测试器.格式化报告致命错误("缺少 usage 时伪造用量: %s", 结果.用量)
			}
			if 正文.关闭次数 != 1 {
				测试器.报告致命错误("响应未释放")
			}
		})
	}
}

func Test本地图片输入并且失败集(测试器 *测试包.测试器) {
	var 图片字节 字节包.缓冲区
	if 错误 := PNG包.编码(&图片字节, 图片包.新建RGBA(图片包.矩形(0, 0, 1, 1))); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	路径 := 文件路径包.合并(测试器.创建临时目录(), "input.png")
	if 错误 := 操作系统包.写入文件(路径, 图片字节.字节(), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已调用 := 0
	客户端 := 新建测试客户端(测试器, func(请求 *HTTP包.请求) (*HTTP包.响应, error) {
		已调用++
		var 通信 struct {
			消息集 []struct {
				内容 []struct {
					类型    string `json:"type"`
					图片URL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(请求.正文).解码(&通信); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if len(通信.消息集) != 1 || len(通信.消息集[0].内容) != 2 || 通信.消息集[0].内容[1].图片URL.URL != "data:image/png;base64,"+基准64包.标准编码.编码目标字符串(图片字节.字节()) {
			测试器.格式化报告致命错误("图片路径未转换为媒体内容: %+v", 通信)
		}
		return 测试响应(请求, "application/json", &已跟踪正文{读取器: 字符串包.新建读取器(`{"choices":[{"index":0,"message":{"role":"assistant","content":"图片已收到"},"finish_reason":"stop"}]}`)}), nil
	})
	请求 := 模型包.请求{模型: "vision", 允许图片输入: true, 消息集: []模型包.消息{{角色: 模型包.角色用户, 内容块集: []模型包.内容块{{类型: "text", 文本: "分析"}, {类型: "image", 路径: 路径}}}}}
	if _, 错误 := 客户端.生成(测试器.上下文(), 请求); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	请求.消息集[0].内容块集[1].路径 = 文件路径包.合并(测试器.创建临时目录(), "missing.png")
	if _, 错误 := 客户端.生成(测试器.上下文(), 请求); !错误判断包.是否属于(错误, 模型包.图片读取错误) {
		测试器.格式化报告致命错误("缺失图片错误不明确: %v", 错误)
	}
	执行上下文, 取消函数 := 上下文包.创建可取消上下文(测试器.上下文())
	取消函数()
	if _, 错误 := 客户端.生成(执行上下文, 请求); !错误判断包.是否属于(错误, 上下文包.已取消错误) {
		测试器.格式化报告致命错误("读取图片忽略取消: %v", 错误)
	}
	if 已调用 != 1 {
		测试器.格式化报告致命错误("错误输入仍调用远端: %d", 已调用)
	}
}
```
