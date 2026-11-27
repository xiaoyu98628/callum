```go
// 包 系统 将数据目录的加密主密钥保存在当前用户的系统凭据库。
package 系统

import 十六进制包 "encoding/hex"

const 服务 = "org.callum.credentials.v1"

// 存储 不持有长期系统连接；每次操作负责取消与释放本次资源。
type 存储 struct{}

func 新建() 存储 { return 存储{} }

func 有效名称(名称 string) bool {
	if len(名称) != 64 {
		return false
	}
	_, 错误 := 十六进制包.解码字符串(名称)
	return 错误 == nil
}
```
