```go
package 执行

import 模型包 "github.com/xiaoyu98628/callum/internal/model"

type 调用参数 struct {
	模型标识 string `json:"model_id"`
	流    *bool  `json:"stream"`
	消息集  []struct {
		角色 模型包.角色  `json:"role"`
		内容 *string `json:"content"`
	} `json:"messages"`
}

type 取消参数 struct {
	执行标识 string `json:"run_id"`
}
```
