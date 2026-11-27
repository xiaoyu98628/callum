```go
// 包 运行时 提供技能所需的本地依赖路径，不安装依赖或执行进程。
package 运行时

import (
	上下文包 "context"
	JSON包 "encoding/json"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type 依赖集工具 struct{ 资源包 *运行时资源包.资源包 }

func 新建(资源包 *运行时资源包.资源包) 工具包.工具 { return 依赖集工具{资源包} }

func (依赖集工具) 获取定义() 工具包.定义 {
	return 工具包.定义{名称: "load_workspace_dependencies", 说明: "返回 Callum 随附 工作区技能的实际 Python、Node、node_modules、原生工具目录和本轮私有工作目录。仅查询已校验的本地资源，不联网、不安装依赖。工作区技能应使用这些绝对路径。", 参数: JSON包.原始JSON消息(`{"type":"object","properties":{},"additionalProperties":false}`), 来源: "builtin", 版本: "1", 影响集: []工具包.影响{工具包.影响读取}}
}

func (依赖集工具) 校验(原始 JSON包.原始JSON消息) (JSON包.原始JSON消息, error) {
	var 值 struct{}
	if _, 错误 := 参数包.解码(原始, &值); 错误 != nil {
		return nil, 错误
	}
	return JSON包.原始JSON消息(`{}`), nil
}

func (当前依赖集工具 依赖集工具) 准备(执行上下文 上下文包.上下文, 范围 工具包.范围, 原始 JSON包.原始JSON消息) (工具包.工具调用, error) {
	return 当前依赖集工具.准备带有权限(执行上下文, 范围, 原始, 工具包.执行权限{})
}

func (当前依赖集工具 依赖集工具) 准备带有权限(执行上下文 上下文包.上下文, _ 工具包.范围, 原始 JSON包.原始JSON消息, 权限 工具包.执行权限) (工具包.工具调用, error) {
	if _, 错误 := 当前依赖集工具.校验(原始); 错误 != nil {
		return nil, 错误
	}
	if !当前依赖集工具.资源包.存在工作区依赖集() {
		return nil, 工具包.不可用错误
	}
	if 错误 := 当前依赖集工具.资源包.检查(执行上下文); 错误 != nil {
		return nil, 工具包.不可用错误
	}
	路径集 := 当前依赖集工具.资源包.工作区路径集()
	路径集["work_directory"] = 权限.工作目录
	输出, 错误 := JSON包.序列化(路径集)
	return 依赖工具调用{输出}, 错误
}

type 依赖工具调用 struct{ 输出 JSON包.原始JSON消息 }

func (依赖工具调用) 获取目标() *工具包.目标 { return nil }
func (依赖工具调用) 关闭() error         { return nil }
func (当前依赖工具调用 依赖工具调用) 执行(执行上下文 上下文包.上下文) (工具包.结果, error) {
	return 工具包.结果{输出: 当前依赖工具调用.输出, 操作结果: 工具包.操作结果无}, 执行上下文.获取错误()
}
```
