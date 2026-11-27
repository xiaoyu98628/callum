```go
package 聊天补全

import (
	错误判断包 "errors"
	HTTP包 "net/http"

	OpenAI包 "github.com/openai/openai-go/v3"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

func 请求标识(响应 *HTTP包.响应) string {
	if 响应 == nil {
		return ""
	}
	return 响应.头部.获取("x-request-id")
}

func 包装错误(操作 string, 响应 *HTTP包.响应, 原因 error) error {
	状态代码 := 0

	if API错误, 成功 := 错误判断包.按类型匹配[*OpenAI包.错误](原因); 成功 {
		状态代码 = API错误.状态代码

		if 响应 == nil {
			响应 = API错误.响应
		}
	}

	if 响应 != nil {
		状态代码 = 响应.状态代码
	}

	return 模型包.新建调用错误(操作, 状态代码, 请求标识(响应), 原因)
}
```
