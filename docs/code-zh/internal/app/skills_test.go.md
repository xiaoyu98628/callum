```go
package 应用模块

import (
	JSON包 "encoding/json"
	格式化包 "fmt"
	输入输出包 "io"
	结构化日志包 "log/slog"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	进程执行包 "os/exec"
	文件路径包 "path/filepath"
	正则表达式包 "regexp"
	切片包 "slices"
	字符串转换包 "strconv"
	字符串包 "strings"
	原子操作包 "sync/atomic"
	测试包 "testing"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	技能包 "github.com/xiaoyu98628/callum/internal/skills"
)

func Test技能列表刷新无暴露正文(测试器 *测试包.测试器) {
	测试器.并行()
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	列表 := func() []技能包.条目 {
		测试器.标记测试辅助函数()
		帧 := 当前对象.调用(测试器, "skills.list", struct{}{})
		var 条目集 []技能包.条目
		if 帧.错误 != nil || JSON包.反序列化(帧.结果, &条目集) != nil || 条目集 == nil {
			测试器.格式化报告致命错误("技能目录结果无效: %+v", 帧)
		}
		if 字符串包.包含(string(帧.结果), "正文哨兵") {
			测试器.报告致命错误("技能列表包含技能正文")
		}
		return 条目集
	}
	if 实际值 := 列表(); len(实际值) != 0 {
		测试器.格式化报告致命错误("空技能目录: %+v", 实际值)
	}
	if 实际值 := 当前对象.调用(测试器, "skills.list", map[string]string{"directory": 测试器.创建临时目录()}); 实际值.错误 == nil || 实际值.错误.代码 != -32602 {
		测试器.格式化报告致命错误("技能目录应拒绝调用方指定路径: %+v", 实际值)
	}
	写入 := func(相对, 说明 string) string {
		测试器.标记测试辅助函数()
		路径 := 文件路径包.合并(启动配置.数据目录, "skills", 文件路径包.来源斜杠(相对), "SKILL.md")
		if 错误 := 操作系统包.创建目录全部(文件路径包.目录(路径), 0700); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 错误 := 操作系统包.写入文件(路径, []byte("---\nname: review\ndescription: "+说明+"\n---\n正文哨兵\n"), 0600); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		实际, 错误 := 文件路径包.求值符号链接集(路径)
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		return 实际
	}
	随附 := 写入(".system/review", "系统示例")
	用户 := 写入("review", "用户版本")
	if 实际值 := 列表(); !切片包.相等(实际值, []技能包.条目{{名称: "review", 说明: "用户版本", 路径: 用户, 来源: "user"}}) {
		测试器.格式化报告致命错误("同名用户技能优先: %+v", 实际值)
	}
	if 错误 := 操作系统包.移除(用户); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 实际值 := 列表(); !切片包.相等(实际值, []技能包.条目{{名称: "review", 说明: "系统示例", 路径: 随附, 来源: "system"}}) {
		测试器.格式化报告致命错误("删除后应恢复系统技能: %+v", 实际值)
	}
}

func Test随附办公文件技能通过智能体并且后续消息(测试器 *测试包.测试器) {
	测试器.并行()
	运行时目录 := 操作系统包.获取环境变量("CALLUM_TEST_RUNTIME_DIR")
	技能目录 := 操作系统包.获取环境变量("CALLUM_TEST_SKILLS_DIR")
	if 运行时目录 == "" || 技能目录 == "" {
		测试器.跳过("真实技能验证需要 CALLUM_TEST_RUNTIME_DIR 与客户端 CALLUM_TEST_SKILLS_DIR")
	}
	Python := 文件路径包.合并(运行时目录, "python", "bin", "python3")
	if 操作系统包.路径分隔符 == '\\' {
		Python = 文件路径包.合并(运行时目录, "python", "python.exe")
	}
	测试夹具集 := 测试器.创建临时目录()
	创建 := 进程执行包.命令上下文(测试器.上下文(), Python, "-I", "-B", "-c", `
from pathlib import Path
import sys
from docx import Document
from openpyxl import Workbook
from pptx import Presentation
p=Path(sys.argv[1])
d=Document(); d.add_paragraph('office-fixture-42'); d.save(p/'report.docx')
w=Workbook(); w.active['A1']='office-fixture-42'; w.save(p/'report.xlsx'); w.close()
r=Presentation(); s=r.slides.add_slide(r.slide_layouts[1]); s.shapes.title.text='office-fixture-42'; r.save(p/'report.pptx')
`, 测试夹具集)
	if 输出, 错误 := 创建.已合并输出(); 错误 != nil {
		测试器.格式化报告致命错误("创建真实 Office 样本: %v %s", 错误, 输出)
	}
	for _, 样本 := range []struct{ 技能, 扩展名, 读取器 string }{{"documents", "docx", "from docx import Document; import sys; print([p.text for p in Document(sys.argv[1]).paragraphs])"}, {"spreadsheets", "xlsx", "from openpyxl import load_workbook; import sys; w=load_workbook(sys.argv[1],read_only=True); print(w.active['A1'].value); w.close()"}, {"presentations", "pptx", "from pptx import Presentation; import sys; print(Presentation(sys.argv[1]).slides[0].shapes.title.text)"}} {
		测试器.执行(样本.技能, func(测试器 *测试包.测试器) {
			测试器.并行()
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			启动配置.运行时目录 = 运行时目录
			启动配置.最大并发执行集 = 1
			if 错误 := 操作系统包.创建目录全部(文件路径包.合并(启动配置.数据目录, "skills"), 0700); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 操作系统包.复制文件系统(文件路径包.合并(启动配置.数据目录, "skills"), 操作系统包.目录文件系统(技能目录)); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			var 调用集 原子操作包.整数32
			var 最后工作 string
			引用 := 正则表达式包.必须编译(`[0-9a-f-]{36}/[0-9a-f-]{36}/[0-9a-f]{64}\.[a-z0-9]+`)
			上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				数据, 错误 := 输入输出包.读取全部(当前请求.正文)
				if 错误 != nil {
					测试器.错误文本(错误)
				}
				var 请求 struct {
					消息集 []struct {
						角色 string         `json:"role"`
						内容 JSON包.原始JSON消息 `json:"content"`
					} `json:"messages"`
				}
				if 错误 := JSON包.反序列化(数据, &请求); 错误 != nil {
					测试器.错误文本(错误)
				}
				数量 := int(调用集.添加(1))
				if 数量 > 4 && !字符串包.包含(string(数据), "followup-skill") {
					测试器.错误文本("新增技能未进入后续请求")
				}
				var 工具内容, 技能上下文 string
				for _, 消息 := range 请求.消息集 {
					if 消息.角色 == "tool" {
						_ = JSON包.反序列化(消息.内容, &工具内容)
					}
					if 消息.角色 == "system" {
						var 内容 string
						if 错误 := JSON包.反序列化(消息.内容, &内容); 错误 != nil {
							测试器.错误文本(错误)
						}
						技能上下文 += 内容
					}
				}
				工具名称 := ""
				var 参数 any
				switch (数量 - 1) % 4 {
				case 0:
					工具名称, 参数 = "read_file", map[string]any{"path": 文件路径包.合并(启动配置.数据目录, "skills", ".system", 样本.技能, "SKILL.md")}
					if !字符串包.包含(技能上下文, "可用技能") || !字符串包.包含(技能上下文, "SKILL.md") {
						测试器.错误文本("模型未收到技能目录")
					}
					if !字符串包.包含(技能上下文, 文件路径包.合并(启动配置.数据目录, "skills", ".system", 样本.技能, "SKILL.md")) {
						测试器.错误文本("当前文件类型的技能未进入目录")
					}
				case 1:
					工具名称, 参数 = "open_attachment", map[string]any{"attachment_ref": 引用.查找字符串(string(数据))}
					if !字符串包.包含(工具内容, "Callum") {
						测试器.错误文本("技能正文未通过工具读取")
					}
				case 2:
					var 值 struct {
						结果 struct {
							路径 string `json:"path"`
							工作 string `json:"work_directory"`
						} `json:"result"`
					}
					if 错误 := JSON包.反序列化([]byte(工具内容), &值); 错误 != nil {
						测试器.错误文本(错误)
					}
					if 值.结果.路径 == "" || 值.结果.工作 == "" {
						测试器.格式化创建错误("未得到附件副本: %s", 工具内容)
					}
					最后工作 = 值.结果.工作
					工具名称, 参数 = "run_command", map[string]any{"command": []string{"python3", "-c", 样本.读取器, 值.结果.路径}, "cwd": 值.结果.工作}
				case 3:
					if !字符串包.包含(工具内容, "office-fixture-42") {
						测试器.格式化创建错误("模型未收到真实文档内容: %s", 工具内容)
					}
				}
				消息 := map[string]any{"role": "assistant", "content": "文档已读取"}
				结束 := "stop"
				if 工具名称 != "" {
					原始, 错误 := JSON包.序列化(参数)
					if 错误 != nil {
						测试器.错误文本(错误)
					}
					消息["tool_calls"] = []any{map[string]any{"id": "call_" + 字符串转换包.整数转字符串(数量), "type": "function", "function": map[string]any{"name": 工具名称, "arguments": string(原始)}}}
					结束 = "tool_calls"
				}
				当前响应写入器.头部().设置("Content-Type", "application/json")
				_ = JSON包.新建编码器(当前响应写入器).编码(map[string]any{"choices": []any{map[string]any{"index": 0, "message": 消息, "finish_reason": 结束}}})
			}))
			defer 上游.关闭()
			运行时, 错误 := 新建测试运行时(测试器.上下文(), 启动配置, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			测试器.注册清理函数(func() {
				if 错误 := 运行时.关闭(); 错误 != nil {
					测试器.错误文本(错误)
				}
			})
			当前对象 := 共享通信对端(测试器, 运行时)
			供应商标识 := 结果字符串(测试器, 当前对象.调用(测试器, "provider.create", map[string]any{"name": "Office", "protocol": "chatcompletions", "base_url": 上游.URL + "/v1"}), "id")
			模型标识 := 结果字符串(测试器, 当前对象.调用(测试器, "model.create", map[string]any{"provider_id": 供应商标识, "display_name": "Office", "remote_model_id": "office", "enabled": true, "context_window": 1000000, "model_type": []string{"text"}}), "id")
			首个 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 模型标识, "stream": false, "content": []map[string]string{{"type": "file", "path": 文件路径包.合并(测试夹具集, "report."+样本.扩展名), "name": "report." + 样本.扩展名}}})
			会话 := 结果字符串(测试器, 首个, "conversation_id")
			当前对象.事件(测试器, "run.completed")
			if _, 错误 := 操作系统包.状态信息(最后工作); !操作系统包.是否不存在(错误) {
				测试器.格式化报告致命错误("执行结束后工作目录未清理: %v", 错误)
			}
			额外 := 文件路径包.合并(启动配置.数据目录, "skills", "followup")
			if 错误 := 操作系统包.创建目录(额外, 0700); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			if 错误 := 操作系统包.写入文件(文件路径包.合并(额外, "SKILL.md"), []byte("---\nname: followup-skill\ndescription: user added skill\n---\n"), 0600); 错误 != nil {
				测试器.报告致命错误(错误)
			}
			要求成功(测试器, 当前对象.调用(测试器, "conversation.send", map[string]any{"conversation_id": 会话, "stream": false, "content": []map[string]string{{"type": "text", "text": 格式化包.格式化字符串("继续读取 %s", 样本.技能)}}}))
			当前对象.事件(测试器, "run.completed")
			if 调用集.加载() != 8 {
				测试器.格式化报告致命错误("工具循环未完整执行: %d", 调用集.加载())
			}
			当前对象.停止(测试器)
		})
	}
}
```
