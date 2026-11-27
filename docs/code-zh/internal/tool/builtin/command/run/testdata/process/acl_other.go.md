```go
//go:build !windows

package main

import 错误判断包 "errors"

func 放宽ACL(string) error { return 错误判断包.新建("ACL 绕过测试仅适用于 Windows") }
```
