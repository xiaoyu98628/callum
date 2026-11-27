```go
package 目录管理

import (
	上下文包 "context"
	JSON包 "encoding/json"

	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	供应商服务包 "github.com/xiaoyu98628/callum/internal/model/provider/service"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// 处理器 将厂商和模型配置接口转换为业务调用，不持有数据库或标准流。
type 处理器 struct{ 供应商集 *供应商服务包.服务 }

// 新建 使用已初始化的非 nil 配置服务创建接口处理器，依赖由调用方持有。
func 新建(供应商集 *供应商服务包.服务) *处理器 { return &处理器{供应商集: 供应商集} }

// 句柄 转换配置接口参数并调用配置业务服务。
func (当前处理器 *处理器) 句柄(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	switch 请求.方法 {
	case "provider.create", "provider.update":
		return 当前处理器.保存供应商(执行上下文, 请求)
	case "provider.get", "provider.delete":
		var 当前对象 标识参数
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if !供应商包.有效标识(当前对象.标识) {
			return nil, 协议包.无效参数()
		}
		if 请求.方法 == "provider.delete" {
			if 错误 := 当前处理器.供应商集.删除供应商(执行上下文, 当前对象.标识); 错误 != nil {
				return nil, 公开错误(错误)
			}
			return 标识结果{当前对象.标识}, nil
		}
		行, 错误 := 当前处理器.供应商集.获取供应商(执行上下文, 当前对象.标识)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		return 查看供应商(行), nil
	case "provider.list":
		var 当前对象 供应商包.列表选项集
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		页, 错误 := 当前处理器.供应商集.列表供应商集(执行上下文, 当前对象)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		输出 := make([]供应商查看, 0, len(页.项集))
		for _, 行 := range 页.项集 {
			输出 = append(输出, 查看供应商(行))
		}
		return 公开页(输出, 页.还有更多, func(当前供应商查看 供应商查看) string { return 当前供应商查看.标识 })
	case "model.create", "model.update":
		return 当前处理器.保存模型(执行上下文, 请求)
	case "model.get", "model.delete":
		var 当前对象 标识参数
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if !供应商包.有效标识(当前对象.标识) {
			return nil, 协议包.无效参数()
		}
		if 请求.方法 == "model.delete" {
			if 错误 := 当前处理器.供应商集.删除模型(执行上下文, 当前对象.标识); 错误 != nil {
				return nil, 公开错误(错误)
			}
			return 标识结果{当前对象.标识}, nil
		}
		行, 错误 := 当前处理器.供应商集.获取模型(执行上下文, 当前对象.标识)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		return 查看模型(行), nil
	case "model.list":
		var 当前对象 struct {
			可选仅 bool `json:"selectable_only"`
			供应商包.列表选项集
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		页, 错误 := 当前处理器.供应商集.列表模型集(执行上下文, 当前对象.可选仅, 当前对象.列表选项集)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		输出 := make([]模型查看, 0, len(页.项集))
		for _, 行 := range 页.项集 {
			输出 = append(输出, 查看模型(行))
		}
		return 公开页(输出, 页.还有更多, func(当前模型查看 模型查看) string { return 当前模型查看.标识 })
	case "model.set_enabled":
		var 当前对象 struct {
			标识      string `json:"id"`
			已启用 *bool  `json:"enabled"`
		}
		if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
			return nil, 错误
		}
		if !供应商包.有效标识(当前对象.标识) || 当前对象.已启用 == nil {
			return nil, 协议包.无效参数()
		}
		if 错误 := 当前处理器.供应商集.设置模型已启用(执行上下文, 当前对象.标识, *当前对象.已启用); 错误 != nil {
			return nil, 公开错误(错误)
		}
		return 标识结果{当前对象.标识}, nil
	default:
		return nil, &协议包.错误{代码: -32601, 消息: "接口不存在"}
	}
}

func (当前处理器 *处理器) 保存供应商(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	var 当前对象 struct {
		标识 string `json:"id"`
		供应商字段集
	}
	if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
		return nil, 错误
	}
	输入, RPC错误 := 当前对象.输入()
	if RPC错误 != nil {
		return nil, RPC错误
	}
	if 请求.方法 == "provider.create" {
		if 当前对象.标识 != "" {
			return nil, 协议包.无效参数()
		}
		标识, 错误 := 当前处理器.供应商集.创建供应商(执行上下文, 输入)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		return 标识结果{标识}, nil
	}
	if !供应商包.有效标识(当前对象.标识) {
		return nil, 协议包.无效参数()
	}
	if 错误 := 当前处理器.供应商集.更新供应商(执行上下文, 当前对象.标识, 输入); 错误 != nil {
		return nil, 公开错误(错误)
	}
	return 标识结果{当前对象.标识}, nil
}

func (当前处理器 *处理器) 保存模型(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	var 当前对象 struct {
		标识 string `json:"id"`
		模型字段集
	}
	if 错误 := 协议包.解码参数(请求.参数, &当前对象); 错误 != nil {
		return nil, 错误
	}
	输入, RPC错误 := 当前对象.输入()
	if RPC错误 != nil {
		return nil, RPC错误
	}
	if 请求.方法 == "model.create" {
		if 当前对象.标识 != "" {
			return nil, 协议包.无效参数()
		}
		标识, 错误 := 当前处理器.供应商集.创建模型(执行上下文, 输入)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		return 标识结果{标识}, nil
	}
	if !供应商包.有效标识(当前对象.标识) {
		return nil, 协议包.无效参数()
	}
	if 错误 := 当前处理器.供应商集.更新模型(执行上下文, 当前对象.标识, 输入); 错误 != nil {
		return nil, 公开错误(错误)
	}
	return 标识结果{当前对象.标识}, nil
}

// 实际序列化后再次限制一页的 JSON 字节，给响应包装和请求标识保留余量。
func 公开页[元素类型 any](项集 []元素类型, 更多 bool, 项标识 func(元素类型) string) (any, *协议包.错误) {
	总计 := 0
	for 索引, 项 := range 项集 {
		原始, 错误 := JSON包.序列化(项)
		if 错误 != nil {
			return nil, 公开错误(错误)
		}
		if 总计+len(原始)+1 > 6<<20 {
			if 索引 == 0 {
				return nil, 公开错误(供应商包.无效配置错误)
			}
			项集, 更多 = 项集[:索引], true
			break
		}
		总计 += len(原始) + 1
	}
	页 := 供应商包.页[元素类型]{项集: 项集, 还有更多: 更多}
	if 更多 {
		页.下一个之后标识 = 项标识(项集[len(项集)-1])
	}
	return 页, nil
}
```
