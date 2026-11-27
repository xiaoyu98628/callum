```go
package 工作区

import (
	上下文包 "context"

	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
	业务域包 "github.com/xiaoyu98628/callum/internal/workspace"
	服务包 "github.com/xiaoyu98628/callum/internal/workspace/service"
)

// 处理器 转换工作区接口，业务服务及数据库由 应用模块 持有。
type 处理器 struct{ 服务 *服务包.服务 }

func 新建(当前服务 *服务包.服务) *处理器 { return &处理器{服务包: 当前服务} }

func (当前处理器 *处理器) 句柄(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	switch 请求.方法 {
	case "workspace.create":
		var 当前对象 struct {
			名称    string   `json:"name"`
			根路径   string   `json:"root_path"`
			其他路径集 []string `json:"other_paths"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		值, 错误 := 当前处理器.服务.创建(执行上下文, 当前对象.名称, 当前对象.根路径, 当前对象.其他路径集...)
		return 值, 公开错误(错误)
	case "workspace.list":
		var 当前对象 业务域包.筛选
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		值, 错误 := 当前处理器.服务.列表(执行上下文, 当前对象)
		return 值, 公开错误(错误)
	case "workspace.get", "workspace.delete", "workspace.restore":
		var 当前对象 struct {
			标识 string `json:"id"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if 请求.方法 == "workspace.get" {
			值, 错误 := 当前处理器.服务.获取(执行上下文, 当前对象.标识)
			return 值, 公开错误(错误)
		}
		值, 错误 := 当前处理器.服务.更新(执行上下文, 当前对象.标识, 业务域包.变更{已删除: new(请求.方法 == "workspace.delete")})
		return 值, 公开错误(错误)
	case "workspace.update":
		var 当前对象 struct {
			标识 string `json:"id"`
			业务域包.变更
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		值, 错误 := 当前处理器.服务.更新(执行上下文, 当前对象.标识, 当前对象.变更)
		return 值, 公开错误(错误)
	default:
		return nil, 协议包.方法未找到()
	}
}
```
