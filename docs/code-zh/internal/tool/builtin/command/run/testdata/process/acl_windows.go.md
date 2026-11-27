```go
package main

import Windows包 "golang.org/x/sys/windows"

// 仅用于隔离测试，尝试通过改变 ACL 绕过执行边界。
func 放宽ACL(路径 string) error {
	安全描述符, 错误 := Windows包.安全描述符来源字符串("D:(A;;FA;;;WD)")
	if 错误 != nil {
		return 错误
	}
	ACL, _, 错误 := 安全描述符.DACL()
	if 错误 != nil {
		return 错误
	}
	return Windows包.设置命名安全信息(路径, Windows包.步骤错误文件对象, Windows包.DACL安全信息, nil, nil, ACL, nil)
}
```
