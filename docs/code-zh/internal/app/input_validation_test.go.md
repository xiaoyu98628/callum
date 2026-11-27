```go
package 应用模块

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	操作系统包 "os"
	文件路径包 "path/filepath"
	测试包 "testing"

	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
)

func Test持久化API集拒绝非RFCUUID版本偏移7(测试器 *测试包.测试器) {
	运行时, _ := 运行时测试夹具(测试器, 1)
	通信对端 := 共享通信对端(测试器, 运行时)
	for _, 方法 := range []string{"provider.get", "model.get", "workspace.get", "conversation.get", "run.get", "step.get", "approval.get"} {
		帧 := 通信对端.调用(测试器, 方法, map[string]string{"id": "01901234-1234-7234-0234-123456789abc"})
		if 帧.错误 == nil || 帧.错误.代码 != -32602 {
			测试器.格式化创建错误("%s 接受了非标准 UUIDv7: %+v", 方法, 帧)
		}
	}
}

func Test无效附件集返回受控输入错误(测试器 *测试包.测试器) {
	运行时, _ := 运行时测试夹具(测试器, 1)
	模型标识 := 直接模型(测试器, 运行时, "https://example.test")
	通信对端 := 共享通信对端(测试器, 运行时)
	目录 := 测试器.创建临时目录()
	超限 := 文件路径包.合并(目录, "oversized.bin")
	文件, 错误 := 操作系统包.创建(超限)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 错误判断包.合并(文件.截取((16<<20)+1), 文件.关闭()); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 路径 := range []string{文件路径包.合并(目录, "missing.txt"), 目录, 超限} {
		帧 := 通信对端.调用(测试器, "conversation.send", map[string]any{"model_id": 模型标识, "content": []map[string]string{{"type": "file", "path": 路径, "name": "input.bin"}}})
		if 帧.错误 == nil || 帧.错误.代码 != 1013 {
			测试器.格式化报告致命错误("附件输入错误被当作服务内部失败: %+v", 帧)
		}
		var 数据 struct {
			类别 string `json:"kind"`
		}
		if JSON包.反序列化(帧.错误.数据, &数据) != nil || 数据.类别 != "attachment_unavailable" {
			测试器.格式化报告致命错误("附件错误缺少分类: %+v", 帧.错误)
		}
	}
	项集, 错误 := 运行时.获取会话集().列表(测试器.上下文(), 会话包.筛选{})
	if 错误 != nil || len(项集) != 0 {
		测试器.格式化报告致命错误("无效附件受理了会话: %+v %v", 项集, 错误)
	}
}
```
