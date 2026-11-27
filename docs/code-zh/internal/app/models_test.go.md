```go
package 应用模块

import (
	字节包 "bytes"
	JSON包 "encoding/json"
	图片包 "image"
	PNG包 "image/png"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
)

func Test图片输入使用已选择模型类型(测试器 *测试包.测试器) {
	for _, 测试 := range []struct {
		名称  string
		能力集 map[string]bool
		类型集 []string
		允许  bool
	}{
		{"capability_without_types", map[string]bool{"image_input": true}, nil, false},
		{"capability_with_text_type", map[string]bool{"image_input": true}, []string{"text"}, false},
		{"image_type_without_capability", nil, []string{"image"}, true},
		{"image_type_with_false_capability", map[string]bool{"image_input": false}, []string{"image"}, true},
		{"image_type_with_capability", map[string]bool{"image_input": true}, []string{"text", "image"}, true},
		{"other_types_without_image", nil, []string{"text", "audio", "video"}, false},
	} {
		测试器.执行(测试.名称, func(测试器 *测试包.测试器) {
			var 请求集 原子操作包.整数32
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				请求集.添加(1)
				var 请求 struct {
					工具集 []JSON包.原始JSON消息 `json:"tools"`
					消息集 []struct {
						角色 string         `json:"role"`
						内容 JSON包.原始JSON消息 `json:"content"`
					} `json:"messages"`
				}
				if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil {
					测试器.错误文本(错误)
					当前响应写入器.写入头部(400)
					return
				}
				if len(请求.工具集) != 9 || len(请求.消息集) != 3 || 请求.消息集[0].角色 != "system" || 请求.消息集[1].角色 != "user" || 请求.消息集[2].角色 != "user" {
					测试器.错误文本("图片请求缺少固定提示或用户消息")
					当前响应写入器.写入头部(400)
					return
				}
				var 部分集 []struct {
					类型    string `json:"type"`
					图片URL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
				if 错误 := JSON包.反序列化(请求.消息集[2].内容, &部分集); 错误 != nil || len(部分集) != 2 || !字符串包.存在前缀(部分集[1].图片URL.URL, "data:image/png;base64,") {
					测试器.错误文本("图片转换或默认工具定义丢失")
				}
				当前响应写入器.头部().设置("Content-Type", "application/json")
				if _, 错误 := 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"图片已收到"},"finish_reason":"stop"}]}`); 错误 != nil {
					测试器.错误文本(错误)
				}
			}))
			测试器.注册清理函数(上游.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.最大并发执行集 = 1
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			进程标识 := 结果字符串(测试器, 当前对象.调用(测试器, "provider.create", map[string]any{"name": "图片厂商", "protocol": "chatcompletions", "base_url": 上游.URL + "/v1"}), "id")
			消息标识 := 结果字符串(测试器, 当前对象.调用(测试器, "model.create", map[string]any{"provider_id": 进程标识, "display_name": "图片模型", "remote_model_id": "vision", "enabled": true, "context_window": 1000000, "capabilities": 测试.能力集, "model_type": 测试.类型集}), "id")
			var 数据 字节包.缓冲区
			if 错误 := PNG包.编码(&数据, 图片包.新建RGBA(图片包.矩形(0, 0, 1, 1))); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			路径 := 文件路径包.合并(测试器.创建临时目录(), "input.png")
			if 错误 := 操作系统包.写入文件(路径, 数据.字节(), 0600); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			帧 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": []map[string]string{{"type": "text", "text": "描述图片"}, {"type": "image", "path": 路径}}})
			if 测试.允许 {
				要求成功(测试器, 帧)
				当前对象.事件(测试器, "run.completed")
				if 请求集.加载() != 1 {
					测试器.报告致命错误("选中图片类型后未完成调用")
				}
			} else if 帧.错误 == nil || 帧.错误.代码 != 1005 || 请求集.加载() != 0 {
				测试器.格式化报告致命错误("未选中图片类型仍被受理或调用失败: %+v", 帧)
			}
			当前对象.停止(测试器)
		})
	}
}
```
