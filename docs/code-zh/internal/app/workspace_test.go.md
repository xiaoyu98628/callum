```go
package 应用模块

import (
	JSON包 "encoding/json"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	切片包 "slices"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	工作区包 "github.com/xiaoyu98628/callum/internal/workspace"
)

func Test工作区文件夹集持久化跨重启(测试器 *测试包.测试器) {
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	一个 := 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "单目录", "root_path": 测试器.创建临时目录()})
	要求成功(测试器, 一个)
	if 值 := 读取工作区(测试器, 一个); 值.其他路径集 == nil || len(值.其他路径集) != 0 {
		测试器.格式化报告致命错误("单目录项目应返回空数组: %s", 一个.结果)
	}

	根, 其他 := 测试器.创建临时目录(), 测试器.创建临时目录()
	别名 := 文件路径包.合并(测试器.创建临时目录(), "alias")
	if 错误 := 操作系统包.符号链接(其他, 别名); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	参数 := map[string]any{"name": "多目录", "root_path": 根, "other_paths": []string{其他, 别名}}
	if 帧 := 当前对象.调用(测试器, "workspace.create", 参数); 帧.错误 == nil || 帧.错误.代码 != -32602 {
		测试器.格式化报告致命错误("没有拒绝指向同一目录的路径: %+v", 帧)
	}
	参数["other_paths"] = []string{别名}
	已创建 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", 参数))
	实际其他, 错误 := 文件路径包.求值符号链接集(其他)
	if 错误 != nil || !切片包.相等(已创建.其他路径集, []string{实际其他}) {
		测试器.格式化报告致命错误("创建结果没有规范化其他目录: %+v %v", 已创建, 错误)
	}
	当前对象.停止(测试器)

	当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	帧 := 当前对象.调用(测试器, "workspace.list", map[string]any{})
	要求成功(测试器, 帧)
	var 值集 []工作区包.工作区
	if 错误 := JSON包.反序列化(帧.结果, &值集); 错误 != nil || len(值集) != 2 {
		测试器.格式化报告致命错误("重启后未读回项目: %s %v", 帧.结果, 错误)
	}
	标识 := map[string]any{"id": 已创建.标识}
	值 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.get", 标识))
	if 值.根路径 != 已创建.根路径 || !切片包.相等(值.其他路径集, 已创建.其他路径集) {
		测试器.格式化报告致命错误("重启后目录发生变化: %+v", 值)
	}
	要求成功(测试器, 当前对象.调用(测试器, "workspace.delete", 标识))
	值 = 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "恢复", "root_path": 已创建.根路径}))
	if 值.标识 != 已创建.标识 || !切片包.相等(值.其他路径集, 已创建.其他路径集) {
		测试器.格式化报告致命错误("省略其他目录恢复时应保留已有目录: %+v", 值)
	}
	要求成功(测试器, 当前对象.调用(测试器, "workspace.delete", 标识))
	值 = 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "恢复为单目录", "root_path": 已创建.根路径, "other_paths": []string{}}))
	if 值.标识 != 已创建.标识 || 值.其他路径集 == nil || len(值.其他路径集) != 0 {
		测试器.格式化报告致命错误("明确指定空数组应清空其他目录: %+v", 值)
	}
}

func 读取工作区(测试器 *测试包.测试器, 帧 通信帧) 工作区包.工作区 {
	测试器.标记测试辅助函数()
	要求成功(测试器, 帧)
	var 值 工作区包.工作区
	if 错误 := JSON包.反序列化(帧.结果, &值); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 值
}

func Test工作区文件夹编辑中通过标准输入输出(测试器 *测试包.测试器) {
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	根, 其他, 已添加 := 测试器.创建临时目录(), 测试器.创建临时目录(), 测试器.创建临时目录()
	已创建 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "原项目", "root_path": 根, "other_paths": []string{其他}}))
	根, 其他 = 已创建.根路径, 已创建.其他路径集[0]
	已添加, 错误 = 文件路径包.求值符号链接集(已添加)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	保留 := 文件路径包.合并(根, "kept.txt")
	if 错误 := 操作系统包.写入文件(保留, []byte("保留磁盘文件"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	变更 := map[string]any{"id": 已创建.标识, "name": "修改项目", "root_path": 其他, "other_paths": []string{已添加}, "expected_folders": []string{根, 其他}}
	值 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.update", 变更))
	if 值.标识 != 已创建.标识 || 值.根路径 != 其他 || !切片包.相等(值.其他路径集, []string{已添加}) {
		测试器.格式化报告致命错误("完整目录更新失败: %+v", 值)
	}
	if 帧 := 当前对象.调用(测试器, "workspace.update", 变更); 帧.错误 == nil || 帧.错误.代码 != 1003 {
		测试器.格式化报告致命错误("旧目录快照覆盖了新设置: %+v", 帧)
	}
	别名 := 文件路径包.合并(测试器.创建临时目录(), "alias")
	if 错误 := 操作系统包.符号链接(其他, 别名); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	变更["expected_folders"], 变更["other_paths"] = []string{其他, 已添加}, []string{别名}
	if 帧 := 当前对象.调用(测试器, "workspace.update", 变更); 帧.错误 == nil || 帧.错误.代码 != -32602 {
		测试器.格式化报告致命错误("接受了重复目录身份: %+v", 帧)
	}
	变更["root_path"], 变更["other_paths"] = "", []string{}
	值 = 读取工作区(测试器, 当前对象.调用(测试器, "workspace.update", 变更))
	if 值.根路径 != "" || len(值.其他路径集) != 0 {
		测试器.格式化报告致命错误("无法移除全部目录: %+v", 值)
	}
	if 内容, 错误 := 操作系统包.读取文件(保留); 错误 != nil || string(内容) != "保留磁盘文件" {
		测试器.格式化报告致命错误("解除目录关联改变了磁盘文件: %q %v", 内容, 错误)
	}
	秒 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "另一个无目录项目"}))
	if 秒.根路径 != "" || 秒.标识 == 值.标识 {
		测试器.格式化报告致命错误("无目录项目发生合并: %+v", 秒)
	}
	当前对象.停止(测试器)
	当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	值 = 读取工作区(测试器, 当前对象.调用(测试器, "workspace.get", map[string]any{"id": 已创建.标识}))
	if 值.根路径 != "" || len(值.其他路径集) != 0 {
		测试器.格式化报告致命错误("重启后无目录状态丢失: %+v", 值)
	}
	读取工作区(测试器, 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 已创建.标识, "root_path": 已添加, "other_paths": []string{}, "expected_folders": []string{}}))
}

func Test工作区主要目录通过标准输入输出(测试器 *测试包.测试器) {
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	根, 其他 := 测试器.创建临时目录(), 测试器.创建临时目录()
	已创建 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "原项目", "root_path": 根, "other_paths": []string{其他}}))
	根, 其他 = 已创建.根路径, 已创建.其他路径集[0]
	文件 := 文件路径包.合并(测试器.创建临时目录(), "file")
	if 错误 := 操作系统包.写入文件(文件, []byte("保留文件"), 0600); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 路径 := range []string{"relative", 文件路径包.合并(测试器.创建临时目录(), "missing"), 文件, 测试器.创建临时目录()} {
		帧 := 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 已创建.标识, "name": "不得保存", "root_path": 路径})
		if 帧.错误 == nil || 帧.错误.代码 != -32602 {
			测试器.格式化报告致命错误("没有拒绝非法或未绑定的主目录 %q: %+v", 路径, 帧)
		}
	}
	冲突 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "占用主目录的项目", "root_path": 其他}))
	for _, 已删除 := range []bool{false, true} {
		if 已删除 {
			要求成功(测试器, 当前对象.调用(测试器, "workspace.delete", map[string]any{"id": 冲突.标识}))
		}
		帧 := 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 已创建.标识, "name": "冲突名称", "root_path": 其他})
		if 帧.错误 == nil || 帧.错误.代码 != 1003 {
			测试器.格式化报告致命错误("没有拒绝与现有项目的主目录冲突: %+v", 帧)
		}
	}
	未变化 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.get", map[string]any{"id": 已创建.标识}))
	if 未变化.名称 != 已创建.名称 || 未变化.根路径 != 根 || !切片包.相等(未变化.其他路径集, []string{其他}) {
		测试器.格式化报告致命错误("被拒绝的更新改变了项目: %+v", 未变化)
	}
	// 独立目录集合可以成功切换，冲突项目仍保留其目录和删除状态。
	主要, 次要 := 测试器.创建临时目录(), 测试器.创建临时目录()
	项目 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "切换项目", "root_path": 主要, "other_paths": []string{次要}}))
	主要, 次要 = 项目.根路径, 项目.其他路径集[0]
	值 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 项目.标识, "name": "切换后名称", "root_path": 次要}))
	if 值.标识 != 项目.标识 || 值.名称 != "切换后名称" || 值.根路径 != 次要 || !切片包.相等(值.其他路径集, []string{主要}) {
		测试器.格式化报告致命错误("没有原子保存名称和主目录: %+v", 值)
	}
	当前对象.停止(测试器)
	当前对象 = 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	值 = 读取工作区(测试器, 当前对象.调用(测试器, "workspace.get", map[string]any{"id": 项目.标识}))
	if 值.根路径 != 次要 || 值.名称 != "切换后名称" || !切片包.相等(值.其他路径集, []string{主要}) {
		测试器.格式化报告致命错误("重启后主目录切换丢失: %+v", 值)
	}
	值 = 读取工作区(测试器, 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 项目.标识, "root_path": 主要}))
	if 值.根路径 != 主要 || !切片包.相等(值.其他路径集, []string{次要}) {
		测试器.格式化报告致命错误("无法切回原主目录: %+v", 值)
	}
}

func Test工作区主要目录使用新建路径基准(测试器 *测试包.测试器) {
	根, 其他 := 测试器.创建临时目录(), 测试器.创建临时目录()
	for 路径, 内容 := range map[string]string{根: "from-original-root", 其他: "from-new-root"} {
		if 错误 := 操作系统包.写入文件(文件路径包.合并(路径, "note.txt"), []byte(内容), 0600); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	var 读取新建根, 无目录 原子操作包.布尔
	绝对文件 := 文件路径包.合并(其他, "note.txt")
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		var 请求 struct {
			工具集 []JSON包.原始JSON消息 `json:"tools"`
			消息集 []struct {
				角色 string `json:"role"`
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil || len(请求.消息集) == 0 {
			测试器.错误文本(错误)
			HTTP包.错误文本(当前响应写入器, "invalid request", HTTP包.状态无效请求)
			return
		}
		回复, 原因 := map[string]any{"content": "完成"}, "stop"
		if len(请求.工具集) > 0 {
			最后 := 请求.消息集[len(请求.消息集)-1]
			if 最后.角色 == "tool" {
				读取新建根.存储(字符串包.包含(最后.内容, "from-new-root") && !字符串包.包含(最后.内容, "from-original-root"))
			} else {
				路径 := "note.txt"
				if 无目录.加载() {
					路径 = 绝对文件
				}
				参数, 错误 := JSON包.序列化(map[string]string{"path": 路径})
				if 错误 != nil {
					测试器.错误文本(错误)
					return
				}
				回复 = map[string]any{"tool_calls": []any{map[string]any{"id": "read-note", "type": "function", "function": map[string]any{"name": "read_file", "arguments": string(参数)}}}}
				原因 = "tool_calls"
			}
		}
		写入文件工具响应(测试器, 当前响应写入器, false, 回复, 原因)
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	消息标识 := 初始化会话模型(测试器, 当前对象, 上游.URL)
	项目 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.create", map[string]any{"name": "相对路径项目", "root_path": 根, "other_paths": []string{其他}}))
	已切换 := 读取工作区(测试器, 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 项目.标识, "root_path": 项目.其他路径集[0]}))
	要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"workspace_id": 项目.标识, "model_id": 消息标识, "stream": false, "content": 消息内容块集("读取 note.txt")}))
	当前对象.事件(测试器, "run.completed")
	if !读取新建根.加载() {
		测试器.报告致命错误("相对路径读取没有使用切换后的主要目录")
	}
	读取新建根.存储(false)
	无目录.存储(true)
	要求成功(测试器, 当前对象.调用(测试器, "workspace.update", map[string]any{"id": 项目.标识, "root_path": "", "other_paths": []string{}, "expected_folders": append([]string{已切换.根路径}, 已切换.其他路径集...)}))
	要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"workspace_id": 项目.标识, "model_id": 消息标识, "stream": false, "content": 消息内容块集("使用绝对路径读取文件")}))
	当前对象.事件(测试器, "run.completed")
	if !读取新建根.加载() {
		测试器.报告致命错误("无目录项目未按未绑定路径规则执行绝对路径工具")
	}
}
```
