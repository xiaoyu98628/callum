```go
// 包 图片输入 负责有界读取及准备本地图片。
package 图片输入

import (
	上下文包 "context"
	错误判断包 "errors"
	输入输出包 "io"
	HTTP包 "net/http"
	操作系统包 "os"
	文件路径包 "path/filepath"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

const (
	最大来源字节   = 16 << 20
	最大来源批次字节 = 32 << 20
	最大图片集    = 8
	最大字节     = 4 << 20
)

type 上下文读取器 struct {
	执行上下文 上下文包.上下文
	读取器   输入输出包.读取器
}

func (当前上下文读取器 上下文读取器) 读取(字节数据 []byte) (int, error) {
	if 错误 := 当前上下文读取器.执行上下文.获取错误(); 错误 != nil {
		return 0, 错误
	}
	return 当前上下文读取器.读取器.读取(字节数据)
}

// 读取 读取规范绝对路径。打开后复核同一个普通文件。
func 读取(执行上下文 上下文包.上下文, 路径 string) ([]byte, string, error) {
	if !文件路径包.是否绝对路径(路径) || 文件路径包.清理(路径) != 路径 {
		return nil, "", 模型包.图片读取错误
	}
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, "", 错误
	}
	信息, 错误 := 操作系统包.状态信息(路径)
	if 错误 != nil {
		return nil, "", 错误判断包.合并(模型包.图片读取错误, 错误)
	}
	if !信息.模式().是否普通文件() {
		return nil, "", 模型包.图片读取错误
	}
	if 信息.大小() > 最大来源字节 {
		return nil, "", 模型包.图片上限错误
	}
	内容, 错误 := 读取文件(执行上下文, 路径, 信息)
	if 错误 != nil {
		return nil, "", 错误
	}
	MIME := HTTP包.检测内容类型(内容)
	switch MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return 内容, MIME, nil
	default:
		return nil, "", 模型包.图片格式错误
	}
}

func 读取文件(执行上下文 上下文包.上下文, 路径 string, 预期 操作系统包.文件信息) (_ []byte, 返回错误 error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return nil, 错误
	}
	文件, 错误 := 操作系统包.打开文件(路径, 读取参数集, 0)
	if 错误 != nil {
		return nil, 错误判断包.合并(模型包.图片读取错误, 错误)
	}
	defer func() {
		if 错误 := 文件.关闭(); 错误 != nil {
			返回错误 = 错误判断包.合并(返回错误, 模型包.图片读取错误, 错误)
		}
	}()
	已打开, 错误 := 文件.状态信息()
	if 错误 != nil {
		return nil, 错误判断包.合并(模型包.图片读取错误, 错误)
	}
	if !已打开.模式().是否普通文件() || !操作系统包.相同文件(预期, 已打开) {
		return nil, 模型包.图片读取错误
	}
	内容, 错误 := 输入输出包.读取全部(输入输出包.上限读取器(上下文读取器{执行上下文, 文件}, 最大来源字节+1))
	if 错误 != nil {
		return nil, 错误判断包.合并(模型包.图片读取错误, 错误)
	}
	if len(内容) > 最大来源字节 {
		return nil, 模型包.图片上限错误
	}
	return 内容, 执行上下文.获取错误()
}
```
