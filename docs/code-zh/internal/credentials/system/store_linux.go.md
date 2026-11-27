```go
//go:build linux

package 系统

import (
	上下文包 "context"
	错误判断包 "errors"
	网络包 "net"
	URL包 "net/url"
	操作系统包 "os"
	字符串包 "strings"

	DBus包 "github.com/godbus/dbus/v5"
	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
)

const 密钥目标 = "org.freedesktop.secrets"
const 密钥服务 = "org.freedesktop.Secret.Service."
const 密钥项 = "org.freedesktop.Secret.Item."

type 密钥 struct {
	会话连接 DBus包.对象路径
	参数   []byte
	值    []byte
	内容类型 string
}

// 每次使用私有连接，不启动桌面服务，不继承共享连接的生命周期。
func 密钥连接(执行上下文 上下文包.上下文) (*DBus包.连接, error) {
	地址 := 操作系统包.获取环境变量("DBUS_SESSION_BUS_ADDRESS")
	if 地址 == "" {
		return nil, 凭据包.不可用错误
	}
	// 显式使用可取消的本地套接字连接；不允许会话地址变成远程凭据传输。
	if !字符串包.存在前缀(地址, "unix:") || 字符串包.包含(地址, ";") {
		return nil, 凭据包.不可用错误
	}
	var 套接字 string
	for _, 部分 := range 字符串包.拆分(字符串包.裁剪前缀(地址, "unix:"), ",") {
		名称, 值, _ := 字符串包.截断(部分, "=")
		if 名称 == "path" || 名称 == "abstract" {
			已解码, 错误 := URL包.路径解除转义(值)
			if 错误 != nil {
				return nil, 凭据包.不可用错误
			}
			套接字 = 已解码
			if 名称 == "abstract" {
				套接字 = "@" + 套接字
			}
		}
	}
	if 套接字 == "" {
		return nil, 凭据包.不可用错误
	}
	var 拨号器 网络包.拨号器
	传输模块, 错误 := 拨号器.拨号上下文(执行上下文, "unix", 套接字)
	if 错误 != nil {
		return nil, 错误判断包.合并(凭据包.不可用错误, 执行上下文.获取错误())
	}
	连接, 错误 := DBus包.新建连接(传输模块, DBus包.带有上下文(执行上下文))
	if 错误 != nil {
		return nil, 错误判断包.合并(凭据包.不可用错误, 执行上下文.获取错误(), 传输模块.关闭())
	}
	if 错误 := 连接.认证(nil); 错误 != nil {
		return nil, 错误判断包.合并(凭据包.不可用错误, 执行上下文.获取错误(), 连接.关闭())
	}
	if 错误 := 连接.问候(); 错误 != nil {
		return nil, 错误判断包.合并(凭据包.不可用错误, 执行上下文.获取错误(), 连接.关闭())
	}
	return 连接, nil
}

func (存储) 加载(执行上下文 上下文包.上下文, 名称 string) (键 []byte, 返回错误 error) {
	if !有效名称(名称) {
		return nil, 凭据包.无效错误
	}
	连接, 错误 := 密钥连接(执行上下文)
	if 错误 != nil {
		return nil, 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 连接.关闭()) }()
	对象 := 连接.对象(密钥目标, "/org/freedesktop/secrets")
	var 已解锁, 已加锁 []DBus包.对象路径
	if 错误 := 对象.调用带有上下文(执行上下文, 密钥服务+"SearchItems", 0, map[string]string{"service": 服务, "account": 名称}).存储(&已解锁, &已加锁); 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	if len(已解锁) == 0 {
		if len(已加锁) > 0 {
			return nil, 凭据包.不可用错误
		}
		return nil, 凭据包.未找到错误
	}
	if len(已解锁) != 1 {
		return nil, 凭据包.无效错误
	}
	var 协商 DBus包.变体
	var 会话连接 DBus包.对象路径
	if 错误 := 对象.调用带有上下文(执行上下文, 密钥服务+"OpenSession", 0, "plain", DBus包.创建变体("")).存储(&协商, &会话连接); 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	var 值 密钥
	if 错误 := 连接.对象(密钥目标, 已解锁[0]).调用带有上下文(执行上下文, 密钥项+"GetSecret", 0, 会话连接).存储(&值); 错误 != nil {
		return nil, 凭据包.不可用错误
	}
	if 值.会话连接 != 会话连接 || len(值.值) != 32 {
		clear(值.值)
		return nil, 凭据包.无效错误
	}
	return 值.值, nil
}

func (存储) 保存(执行上下文 上下文包.上下文, 名称 string, 键 []byte) (返回错误 error) {
	if !有效名称(名称) || len(键) != 32 {
		return 凭据包.无效错误
	}
	连接, 错误 := 密钥连接(执行上下文)
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 连接.关闭()) }()
	对象 := 连接.对象(密钥目标, "/org/freedesktop/secrets")
	var 协商 DBus包.变体
	var 会话连接, 集合 DBus包.对象路径
	if 错误 := 对象.调用带有上下文(执行上下文, 密钥服务+"OpenSession", 0, "plain", DBus包.创建变体("")).存储(&协商, &会话连接); 错误 != nil {
		return 凭据包.不可用错误
	}
	if 错误 := 对象.调用带有上下文(执行上下文, 密钥服务+"ReadAlias", 0, "default").存储(&集合); 错误 != nil || 集合 == "/" {
		return 凭据包.不可用错误
	}
	属性集 := map[string]DBus包.变体{
		密钥项 + "Label":      DBus包.创建变体("Callum 加密主密钥"),
		密钥项 + "Attributes": DBus包.创建变体(map[string]string{"service": 服务, "account": 名称}),
	}
	var 项, 提示词 DBus包.对象路径
	if 错误 := 连接.对象(密钥目标, 集合).调用带有上下文(执行上下文, "org.freedesktop.Secret.Collection.CreateItem", 0, 属性集, 密钥{会话连接, []byte{}, 键, "application/octet-stream"}, true).存储(&项, &提示词); 错误 != nil || 提示词 != "/" || 项 == "/" {
		return 凭据包.不可用错误
	}
	return 执行上下文.获取错误()
}
```
