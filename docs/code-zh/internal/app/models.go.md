```go
package 应用模块

import (
	上下文包 "context"
	HTTP包 "net/http"
	切片包 "slices"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	附件包 "github.com/xiaoyu98628/callum/internal/attachment"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	聊天补全包 "github.com/xiaoyu98628/callum/internal/model/adapter/chatcompletions"
	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	供应商服务包 "github.com/xiaoyu98628/callum/internal/model/provider/service"
)

func 新建模型客户端(运行时 供应商包.运行时配置, 客户端 *HTTP包.客户端, 读取附件 func(上下文包.上下文, string) ([]byte, string, error)) (模型包.客户端, error) {
	if 运行时.协议 != "chatcompletions" {
		return nil, 推理调用包.不支持错误
	}
	return 聊天补全包.新建(聊天补全包.配置{基础URL: 运行时.基础URL, API密钥: 运行时.API密钥, 读取附件: 读取附件}, 客户端)
}

// 新建模型解析器 将配置业务与具体适配器连接，不持有数据库查询或网络调用事务。
func 新建模型解析器(供应商集 *供应商服务包.服务, 客户端 *HTTP包.客户端) 推理调用包.解析器 {
	return func(执行上下文 上下文包.上下文, 标识 string) (模型包.客户端, string, error) {
		运行时, 错误 := 供应商集.解析(执行上下文, 标识)
		if 错误 != nil {
			return nil, "", 错误
		}
		适配器, 错误 := 新建模型客户端(运行时, 客户端, nil)
		if 错误 != nil {
			return nil, "", 错误
		}
		return 适配器, 运行时.模型.远程模型标识, nil
	}
}

func 新建会话解析器(供应商集 *供应商服务包.服务, 客户端 *HTTP包.客户端, 附件集 *附件包.存储) 智能体包.解析器 {
	return func(执行上下文 上下文包.上下文, 标识 string) (智能体包.已解析, error) {
		运行时, 错误 := 供应商集.解析(执行上下文, 标识)
		if 错误 != nil {
			return 智能体包.已解析{}, 错误
		}
		适配器, 错误 := 新建模型客户端(运行时, 客户端, 附件集.读取)
		if 错误 != nil {
			return 智能体包.已解析{}, 错误
		}
		已解析 := 智能体包.已解析{客户端: 适配器, 协议: 运行时.协议, 图片输入: 切片包.包含(运行时.模型.模型类型, "image"), 快照: 模型包.快照{标识: 运行时.模型.标识, 显示名称: 运行时.模型.显示名称, 远程模型标识: 运行时.模型.远程模型标识, 供应商: 模型包.供应商快照{标识: 运行时.模型.供应商标识, 名称: 运行时.供应商名称}}}
		if 运行时.模型.上下文窗口 != nil {
			已解析.上下文窗口 = *运行时.模型.上下文窗口
		}
		if 运行时.模型.最大输出词元 != nil {
			已解析.最大输出词元 = *运行时.模型.最大输出词元
		}
		return 已解析, nil
	}
}
```
