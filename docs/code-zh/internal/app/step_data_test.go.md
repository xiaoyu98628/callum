```go
package 应用模块

import (
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"
	UTF8包 "unicode/utf8"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
)

func Test大型模型结果使用有界UTF8片段集(测试器 *测试包.测试器) {
	// 各用例独占临时目录、运行时及受控端点，可与其他应用回归并行。
	测试器.并行()
	大型 := 字符串包.重复("源", 3<<20)
	推理 := 字符串包.重复("a", (64<<10)-2) + "🙂" + 字符串包.重复("中", 1024)
	推理JSON, 错误 := JSON包.序列化(推理)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		当前响应写入器.头部().设置("Content-Type", "application/json")
		// 原始结果超过单条通信限制，但模型正文很短；步骤通知必须只发送元数据。
		if _, 错误 := 输入输出包.写入字符串(当前响应写入器, `{"provider_extra":"`+大型+`","choices":[{"index":0,"message":{"role":"assistant","content":"完成","reasoning_content":`+string(推理JSON)+`},"finish_reason":"stop"}]}`); 错误 != nil && 当前请求.上下文().获取错误() == nil {
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
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("原始结果测试")})
	执行标识 := 结果字符串(测试器, 已受理, "run_id")
	// 本用例解析并保存 9 MiB 原始响应；竞态 下可超过普通请求的 3 秒测试期限。
	// 只调整该连接的有界读取预算，生产超时、大小和 UTF-8 分片断言保持不变。
	当前对象.读取超时 = 10 * 时间包.秒
	当前对象.事件(测试器, "run.completed")
	步骤标识 := 结果字符串来源步骤集(测试器, 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识}))
	首个 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "output"})
	要求成功(测试器, 首个)
	type 窗口 struct {
		片段    string `json:"fragment"`
		总计字节  int    `json:"total_bytes"`
		完成    bool   `json:"done"`
		下一个偏移 int    `json:"next_offset"`
		存在    bool   `json:"present"`
	}
	var 分片 窗口
	if 错误 := JSON包.反序列化(首个.结果, &分片); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 分片.总计字节 <= 8<<20 || 分片.完成 || len(分片.片段) > 64<<10 || !UTF8包.有效字符串(分片.片段) {
		测试器.格式化报告致命错误("结果没有分段: bytes=%d next=%d", 分片.总计字节, 分片.下一个偏移)
	}
	预览 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "text"})
	要求成功(测试器, 预览)
	var 文本 窗口
	if 错误 := JSON包.反序列化(预览.结果, &文本); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 预期 := (窗口{片段: "完成", 总计字节: len("完成"), 下一个偏移: len("完成"), 完成: true, 存在: true}); 文本 != 预期 {
		测试器.格式化报告致命错误("大型原始结果的正文投影: %+v，应为 %+v", 文本, 预期)
	}
	偏移 := 0
	for !字符串包.包含(分片.片段, "源") {
		if 分片.完成 || 分片.下一个偏移 <= 偏移 {
			测试器.报告致命错误("未保存原始供应商字段")
		}
		偏移 = 分片.下一个偏移
		页 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "output", "offset": 偏移})
		要求成功(测试器, 页)
		if 错误 := JSON包.反序列化(页.结果, &分片); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	启动 := 偏移 + 字符串包.索引(分片.片段, "源")
	部分 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "output", "offset": 启动, "limit": 4})
	要求成功(测试器, 部分)
	if 错误 := JSON包.反序列化(部分.结果, &分片); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 分片.片段 != "源" || 分片.下一个偏移 != 启动+3 {
		测试器.格式化报告致命错误("UTF-8 字符被截断: %+v", 分片)
	}
	无效 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "output", "offset": 启动 + 1, "limit": 4})
	if 无效.错误 == nil || 无效.错误.代码 != -32602 {
		测试器.格式化报告致命错误("允许从字符中间开始: %+v", 无效)
	}
	var 已保存 字符串包.构建器
	for 偏移 := 0; ; {
		页 := 当前对象.调用(测试器, "step.data", map[string]any{"step_id": 步骤标识, "field": "reasoning", "offset": 偏移})
		要求成功(测试器, 页)
		if 错误 := JSON包.反序列化(页.结果, &分片); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 分片.总计字节 != len(推理) || 分片.下一个偏移 <= 偏移 || len(分片.片段) > 64<<10 || !UTF8包.有效字符串(分片.片段) {
			测试器.格式化报告致命错误("思考分页无效: bytes=%d offset=%d next=%d", 分片.总计字节, 偏移, 分片.下一个偏移)
		}
		已保存.写入字符串(分片.片段)
		if 分片.完成 {
			break
		}
		偏移 = 分片.下一个偏移
	}
	if 已保存.字符串() != 推理 {
		测试器.格式化报告致命错误("思考内容不完整: bytes=%d，应为 %d", 已保存.长度(), len(推理))
	}
	要求成功(测试器, 当前对象.调用(测试器, "run.get", map[string]any{"id": 执行标识}))
	当前对象.停止(测试器)
}
```
