```go
package 技能包

import (
	上下文包 "context"
	JSON包 "encoding/json"

	技能包 "github.com/xiaoyu98628/callum/internal/skills"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 处理器 将只读技能目录转换为协议响应，目录由运行时持有。
type 处理器 struct{ 存储 *技能包.存储 }

// 新建 接收运行时已经打开的非 nil 技能目录。
func 新建(存储 *技能包.存储) *处理器 { return &处理器{存储: 存储} }

// 列表 只读取已发现技能的元数据。
func (当前处理器 *处理器) 列表(执行上下文 上下文包.上下文, 请求 协议包.请求) (JSON包.原始JSON消息, *协议包.错误) {
	if 错误 := 协议包.解码参数(请求.参数, &struct{}{}); 错误 != nil {
		return nil, 错误
	}
	数据, 错误 := 当前处理器.存储.目录管理(执行上下文)
	if 错误 != nil {
		return nil, &协议包.错误{代码: -32603, 消息: "无法读取技能列表"}
	}
	return 数据, nil
}
```
