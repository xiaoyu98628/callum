```go
package 工作区

import 上下文包 "context"

// 仓储 实现工作区持久化；调用返回前释放连接，变更与执行检查必须原子完成。
type 仓储 interface {
	创建(上下文包.上下文, 工作区) (工作区, error)
	获取(上下文包.上下文, string) (工作区, error)
	列表(上下文包.上下文, 筛选) ([]工作区, error)
	更新(上下文包.上下文, string, 变更) (工作区, error)
}
```
