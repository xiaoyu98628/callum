```go
package 应用模块

import (
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	同步包 "sync"
	测试包 "testing"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func Test技能引用集通过发送编辑重发并且重启(测试器 *测试包.测试器) {
	测试器.并行()
	var 互斥锁 同步包.互斥锁
	var 请求集 []string
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		数据, 错误 := 输入输出包.读取全部(当前请求.正文)
		if 错误 != nil {
			测试器.错误文本(错误)
			return
		}
		互斥锁.加锁()
		请求集 = append(请求集, string(数据))
		互斥锁.解锁()
		当前响应写入器.头部().设置("Content-Type", "application/json")
		_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
	}))
	defer 上游.关闭()
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	目录 := 文件路径包.合并(启动配置.数据目录, "skills", "review")
	if 错误 := 操作系统包.创建目录全部(目录, 0700); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	路径 := 文件路径包.合并(目录, "SKILL.md")
	if 错误 := 操作系统包.写入文件(路径, []byte("---\nname: Review\ndescription: explicit review\n---\n核对任务。"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	路径, 错误 = 文件路径包.求值符号链接集(路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	内容 := []模型包.内容块{{类型: "skill", 名称: "Review", 路径: 路径}, {类型: "text", 文本: "核对正文"}}
	for _, 无效 := range []模型包.内容块{
		{类型: "skill", 名称: "Review", 路径: 文件路径包.合并(测试器.创建临时目录(), "SKILL.md")},
		{类型: "skill", 名称: "wrong-name", 路径: 路径},
	} {
		无效 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "content": []模型包.内容块{无效, 内容[1]}})
		if 无效.错误 == nil || 无效.错误.代码 != -32602 {
			测试器.格式化报告致命错误("非法技能被受理: %+v", 无效)
		}
	}
	已发送 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 内容})
	会话标识, 用户标识 := 结果字符串(测试器, 已发送, "conversation_id"), 结果字符串(测试器, 已发送, "user_message_id")
	当前对象.事件(测试器, "run.completed")
	行集 := 读取消息集(测试器, 当前对象, 会话标识)
	if len(行集) != 2 || len(行集[0].内容) != 2 || 行集[0].内容[0] != 内容[0] || 行集[0].内容[1] != 内容[1] {
		测试器.格式化报告致命错误("技能未独立保存: %+v", 行集)
	}
	内容[1].文本 = "修改后的正文"
	编辑重发 := 当前对象.调用(测试器, "conversation.resend", map[string]any{"conversation_id": 会话标识, "message_id": 用户标识, "stream": false, "content": 内容})
	要求成功(测试器, 编辑重发)
	当前对象.事件(测试器, "run.completed")
	当前对象.停止(测试器)
	当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	行集 = 读取消息集(测试器, 当前对象, 会话标识)
	if 行集[0].内容[0] != 内容[0] || 行集[0].内容[1] != 内容[1] {
		测试器.格式化报告致命错误("重启后引用丢失: %+v", 行集)
	}
	互斥锁.加锁()
	已合并 := 字符串包.合并(请求集, "\n")
	互斥锁.解锁()
	if !字符串包.包含(已合并, "$Review 核对正文") || !字符串包.包含(已合并, "$Review 修改后的正文") || 字符串包.包含(已合并, "请使用技能") {
		测试器.格式化报告致命错误("模型引用格式错误: %s", 已合并)
	}
	var 已见 bool
	for _, 请求 := range 请求集 {
		var 已解析 struct {
			消息集 []struct {
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if JSON包.反序列化([]byte(请求), &已解析) != nil {
			continue
		}
		for _, 消息 := range 已解析.消息集 {
			if 字符串包.包含(消息.内容, `"skills":[`) && 字符串包.包含(消息.内容, 路径) {
				已见 = true
			}
		}
	}
	if !已见 {
		测试器.报告致命错误("模型缺少所选技能的定位元数据")
	}
	if 错误 := 操作系统包.移除(路径); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	无效 := 当前对象.调用(测试器, "conversation.resend", map[string]any{"conversation_id": 会话标识, "message_id": 用户标识, "content": 内容})
	if 无效.错误 == nil || 无效.错误.代码 != -32602 {
		测试器.格式化报告致命错误("删除后的技能仍可重发: %+v", 无效)
	}
}
```
