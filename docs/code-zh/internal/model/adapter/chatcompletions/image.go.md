```go
package 聊天补全

import (
	上下文包 "context"
	基准64包 "encoding/base64"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
	图片输入包 "github.com/xiaoyu98628/callum/internal/model/imageinput"
)

// 最大输入字节 限制包含内联图片的实际请求 JSON。
const 最大输入字节 = 12 << 20

// 按已校验的引用缓存准备结果。适配器实例归属单次执行。
func (当前客户端 *客户端) 图片数据(执行上下文 上下文包.上下文, 内容块 模型包.内容块, 已使用 map[string]string) (string, error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return "", 错误
	}
	var 数据 []byte
	var MIME string
	var 错误 error
	键 := 内容块.附件引用
	if 键 != "" {
		if 当前客户端.读取附件 == nil {
			return "", 模型包.图片读取错误
		}
		if 已编码, 成功 := 已使用[键]; 成功 {
			return 已编码, nil
		}
		if 已编码, 成功 := 当前客户端.图片集[键]; 成功 {
			已使用[键] = 已编码
			return 已编码, nil
		}
		数据, MIME, 错误 = 当前客户端.读取附件(执行上下文, 键)
	} else {
		数据, MIME, 错误 = 图片输入包.读取(执行上下文, 内容块.路径)
	}
	if 错误 != nil {
		return "", 错误
	}
	数据, 错误 = 图片输入包.准备(执行上下文, 数据, MIME)
	if 错误 != nil {
		return "", 错误
	}
	已编码 := "data:" + MIME + ";base64," + 基准64包.标准编码.编码目标字符串(数据)
	if 键 != "" {
		已使用[键] = 已编码
	}
	return 已编码, nil
}
```
