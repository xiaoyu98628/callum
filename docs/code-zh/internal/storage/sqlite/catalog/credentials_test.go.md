```go
package 目录管理测试

import (
	字节包 "bytes"
	上下文包 "context"
	错误判断包 "errors"
	操作系统包 "os"
	文件路径包 "path/filepath"
	测试包 "testing"

	UUID包 "github.com/google/uuid"
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	凭据测试包 "github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
	目录管理包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

type 失败中密码算法 struct {
	凭据包.密码算法
	调用集 int
}

func (当前失败中密码算法 *失败中密码算法) 加密封装(执行上下文 上下文包.上下文, 标识, 文本 string) (string, error) {
	当前失败中密码算法.调用集++
	if 当前失败中密码算法.调用集 == 2 {
		return "", 错误判断包.新建("受控加密失败")
	}
	return 当前失败中密码算法.密码算法.加密封装(执行上下文, 标识, 文本)
}

func Test凭据迁移回滚返回并且移除受控明文(测试器 *测试包.测试器) {
	for _, 模式 := range []string{"delete", "wal"} {
		测试器.执行(模式, func(测试器 *测试包.测试器) { 测试凭据迁移(测试器, 模式) })
	}
}

func 测试凭据迁移(测试器 *测试包.测试器, 模式 string) {
	路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 数据库.关闭()
	if 错误 := 数据库.数据库.进程执行("PRAGMA journal_mode=" + 模式).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 键 := range []string{"legacy-first-private-value", "legacy-second-private-value"} {
		行 := 表结构包.供应商{标识: UUID包.必须(UUID包.新建UUIDv7()).字符串(), 名称: "旧厂商", 协议: "chatcompletions", 基础URL: "https://example.invalid", API密钥: 键}
		if 错误 := 数据库.数据库.选择("ID", "Name", "Protocol", "BaseURL", "APIKey", "CredentialFormat").创建(&行).错误; 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 错误 := 数据库.数据库.模型(&行).更新("credential_format", 0).错误; 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	值, _ := 凭据包.新建(凭据测试包.系统凭据库{}, 路径)
	defer 值.关闭()
	if 错误 := 目录管理包.迁移凭据(测试器.上下文(), 数据库.数据库, &失败中密码算法{密码算法: 值}); 错误 == nil {
		测试器.报告致命错误("未产生受控迁移失败")
	}
	var 数量 int64
	if 错误 := 数据库.数据库.模型(&表结构包.供应商{}).条件("credential_format=0 AND api_key LIKE 'legacy-%'").数量(&数量).错误; 错误 != nil || 数量 != 2 {
		测试器.报告致命错误("迁移失败未整体回滚", 数量, 错误)
	}
	if 错误 := 目录管理包.迁移凭据(测试器.上下文(), 数据库.数据库, 值); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 目录管理包.迁移凭据(测试器.上下文(), 数据库.数据库, 值); 错误 != nil {
		测试器.报告致命错误("重复迁移失败", 错误)
	}
	var 行集 []表结构包.供应商
	if 错误 := 数据库.数据库.查找(&行集).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 行 := range 行集 {
		if 行.凭据格式 != 1 {
			测试器.报告致命错误("未记录密文格式")
		}
		值, 错误 := 值.打开(测试器.上下文(), 行.标识, 行.API密钥)
		if 错误 != nil || !字节包.存在前缀([]byte(值), []byte("legacy-")) {
			测试器.报告致命错误("迁移丢失凭据", 错误)
		}
	}
	// 连接仍打开时检查主文件及 WAL，避免仅在关闭连接后才碰巧清除旧页。
	for _, 文件 := range []string{路径, 路径 + "-wal"} {
		原始, 错误 := 操作系统包.读取文件(文件)
		if 错误判断包.是否属于(错误, 操作系统包.不存在错误) && 文件 != 路径 {
			continue
		}
		if 错误 != nil || 字节包.包含(原始, []byte("legacy-first-private-value")) || 字节包.包含(原始, []byte("legacy-second-private-value")) {
			测试器.报告致命错误("数据库仍含可控的明文残留", 文件, 错误)
		}
	}
}

type 已加锁已有密码算法 struct {
	凭据包.密码算法
	读取  int
	已准备 bool
}

func (当前已加锁已有密码算法 *已加锁已有密码算法) 打开(执行上下文 上下文包.上下文, 标识, 值 string) (string, error) {
	当前已加锁已有密码算法.读取++
	if 当前已加锁已有密码算法.读取 == 2 {
		return "", 凭据包.不可用错误
	}
	return 当前已加锁已有密码算法.密码算法.打开(执行上下文, 标识, 值)
}
func (当前已加锁已有密码算法 *已加锁已有密码算法) 准备(上下文包.上下文) error {
	当前已加锁已有密码算法.已准备 = true
	return 凭据包.不可用错误
}

func Test迁移检查集每个已有键之前准备中旧版凭据(测试器 *测试包.测试器) {
	路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 数据库.关闭()
	凭据保险库, _ := 凭据包.新建(凭据测试包.系统凭据库{}, 路径)
	defer 凭据保险库.关闭()
	for 数量 := range 3 {
		行 := 表结构包.供应商{标识: UUID包.必须(UUID包.新建UUIDv7()).字符串(), 名称: "迁移保护", 协议: "chatcompletions", 基础URL: "https://example.invalid", API密钥: "legacy-kept-on-failure"}
		if 数量 < 2 {
			行.API密钥, 错误 = 凭据保险库.加密封装(测试器.上下文(), 行.标识, "existing-private-value")
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
		}
		if 错误 := 数据库.数据库.创建(&行).错误; 错误 != nil {
			测试器.报告致命错误(错误)
		}
		if 数量 == 2 {
			if 错误 := 数据库.数据库.模型(&行).更新("credential_format", 0).错误; 错误 != nil {
				测试器.报告致命错误(错误)
			}
		}
	}
	已加锁 := &已加锁已有密码算法{密码算法: 凭据保险库}
	if 错误 := 目录管理包.迁移凭据(测试器.上下文(), 数据库.数据库, 已加锁); !错误判断包.是否属于(错误, 凭据包.不可用错误) {
		测试器.报告致命错误(错误)
	}
	if 已加锁.已准备 || 已加锁.读取 != 2 {
		测试器.报告致命错误("已有密钥不可用时尝试创建替代密钥")
	}
	var 数量 int64
	if 错误 := 数据库.数据库.模型(&表结构包.供应商{}).条件("credential_format=0 AND api_key=?", "legacy-kept-on-failure").数量(&数量).错误; 错误 != nil || 数量 != 1 {
		测试器.报告致命错误("迁移失败改变旧凭据", 数量, 错误)
	}
}
```
