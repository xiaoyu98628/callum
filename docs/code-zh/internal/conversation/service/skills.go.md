```go
package 服务

import (
	上下文包 "context"
	格式化包 "fmt"
	切片包 "slices"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	技能包 "github.com/xiaoyu98628/callum/internal/skills"
)

// 受理只核对已安装目录，客户端路径不能使任意文件成为技能入口。
func (当前服务 *服务) 校验技能(执行上下文 上下文包.上下文, 内容 []模型包.内容块) error {
	索引 := 切片包.索引函数(内容, func(内容块 模型包.内容块) bool { return 内容块.类型 == "skill" })
	if 索引 < 0 {
		return nil
	}
	条目集, 错误 := 当前服务.技能.快照(执行上下文)
	if 错误 != nil {
		return 格式化包.格式化创建错误("%w: 读取技能目录: %w", 智能体包.存储错误, 错误)
	}
	已选择 := 内容[索引]
	if !切片包.包含函数(条目集, func(条目 技能包.条目) bool { return 条目.名称 == 已选择.名称 && 条目.路径 == 已选择.路径 }) {
		return 会话包.无效错误
	}
	return nil
}
```
