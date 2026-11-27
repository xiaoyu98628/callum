```go
//go:build darwin

package 系统

import (
	字节包 "bytes"
	上下文包 "context"
	基准64包 "encoding/base64"
	错误判断包 "errors"
	进程执行包 "os/exec"
	字符串包 "strings"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
)

// 主密钥通过 标准输入 交给 安全 的交互解析器，不出现在命令参数或环境中。
func (存储) 保存(执行上下文 上下文包.上下文, 名称 string, 键 []byte) error {
	if !有效名称(名称) || len(键) != 32 {
		return 凭据包.无效错误
	}
	命令 := 进程执行包.命令上下文(执行上下文, "/usr/bin/security", "-i")
	命令.标准输入 = 字符串包.新建读取器("add-generic-password -U -s " + 服务 + " -a " + 名称 + " -w " + 基准64包.标准编码.编码目标字符串(键) + "\n")
	if 错误 := 命令.执行(); 错误 != nil {
		return 错误判断包.合并(凭据包.不可用错误, 执行上下文.获取错误())
	}
	return nil
}

type 有界输出 struct{ 字节包.缓冲区 }

func (当前有界输出 *有界输出) 写入(字节数据 []byte) (int, error) {
	if 当前有界输出.长度()+len(字节数据) > 4096 {
		return 0, 凭据包.无效错误
	}
	return 当前有界输出.缓冲区.写入(字节数据)
}

func (存储) 加载(执行上下文 上下文包.上下文, 名称 string) ([]byte, error) {
	if !有效名称(名称) {
		return nil, 凭据包.无效错误
	}
	命令 := 进程执行包.命令上下文(执行上下文, "/usr/bin/security", "find-generic-password", "-s", 服务, "-a", 名称, "-w")
	var 输出 有界输出
	命令.标准输出 = &输出
	if 错误 := 命令.执行(); 错误 != nil {
		if 执行上下文.获取错误() != nil {
			return nil, 执行上下文.获取错误()
		}
		if 退出, 成功 := 错误判断包.按类型匹配[*进程执行包.退出错误](错误); 成功 && 退出.退出码() == 44 {
			return nil, 凭据包.未找到错误
		}
		return nil, 凭据包.不可用错误
	}
	键, 错误 := 基准64包.标准编码.解码字符串(字符串包.裁剪空间(输出.字符串()))
	if 错误 != nil || len(键) != 32 {
		clear(键)
		return nil, 凭据包.无效错误
	}
	return 键, nil
}
```
