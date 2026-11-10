//go:build linux

package system

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/xiaoyu98628/callum/internal/credentials"
)

const secretDestination = "org.freedesktop.secrets"
const secretService = "org.freedesktop.Secret.Service."
const secretItem = "org.freedesktop.Secret.Item."

type secret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// 每次使用私有连接，不启动桌面服务，不继承共享连接的生命周期。
func secretConnection(ctx context.Context) (*dbus.Conn, error) {
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		return nil, credentials.ErrUnavailable
	}
	// 显式使用可取消的本地套接字连接；不允许会话地址变成远程凭据传输。
	if !strings.HasPrefix(address, "unix:") || strings.Contains(address, ";") {
		return nil, credentials.ErrUnavailable
	}
	var socket string
	for _, part := range strings.Split(strings.TrimPrefix(address, "unix:"), ",") {
		name, value, _ := strings.Cut(part, "=")
		if name == "path" || name == "abstract" {
			decoded, err := url.PathUnescape(value)
			if err != nil {
				return nil, credentials.ErrUnavailable
			}
			socket = decoded
			if name == "abstract" {
				socket = "@" + socket
			}
		}
	}
	if socket == "" {
		return nil, credentials.ErrUnavailable
	}
	var dialer net.Dialer
	transport, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, errors.Join(credentials.ErrUnavailable, ctx.Err())
	}
	conn, err := dbus.NewConn(transport, dbus.WithContext(ctx))
	if err != nil {
		return nil, errors.Join(credentials.ErrUnavailable, ctx.Err(), transport.Close())
	}
	if err := conn.Auth(nil); err != nil {
		return nil, errors.Join(credentials.ErrUnavailable, ctx.Err(), conn.Close())
	}
	if err := conn.Hello(); err != nil {
		return nil, errors.Join(credentials.ErrUnavailable, ctx.Err(), conn.Close())
	}
	return conn, nil
}

func (Store) Load(ctx context.Context, name string) (key []byte, retErr error) {
	if !validName(name) {
		return nil, credentials.ErrInvalid
	}
	conn, err := secretConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, conn.Close()) }()
	object := conn.Object(secretDestination, "/org/freedesktop/secrets")
	var unlocked, locked []dbus.ObjectPath
	if err := object.CallWithContext(ctx, secretService+"SearchItems", 0, map[string]string{"service": service, "account": name}).Store(&unlocked, &locked); err != nil {
		return nil, credentials.ErrUnavailable
	}
	if len(unlocked) == 0 {
		if len(locked) > 0 {
			return nil, credentials.ErrUnavailable
		}
		return nil, credentials.ErrNotFound
	}
	if len(unlocked) != 1 {
		return nil, credentials.ErrInvalid
	}
	var negotiation dbus.Variant
	var session dbus.ObjectPath
	if err := object.CallWithContext(ctx, secretService+"OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&negotiation, &session); err != nil {
		return nil, credentials.ErrUnavailable
	}
	var value secret
	if err := conn.Object(secretDestination, unlocked[0]).CallWithContext(ctx, secretItem+"GetSecret", 0, session).Store(&value); err != nil {
		return nil, credentials.ErrUnavailable
	}
	if value.Session != session || len(value.Value) != 32 {
		clear(value.Value)
		return nil, credentials.ErrInvalid
	}
	return value.Value, nil
}

func (Store) Save(ctx context.Context, name string, key []byte) (retErr error) {
	if !validName(name) || len(key) != 32 {
		return credentials.ErrInvalid
	}
	conn, err := secretConnection(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, conn.Close()) }()
	object := conn.Object(secretDestination, "/org/freedesktop/secrets")
	var negotiation dbus.Variant
	var session, collection dbus.ObjectPath
	if err := object.CallWithContext(ctx, secretService+"OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&negotiation, &session); err != nil {
		return credentials.ErrUnavailable
	}
	if err := object.CallWithContext(ctx, secretService+"ReadAlias", 0, "default").Store(&collection); err != nil || collection == "/" {
		return credentials.ErrUnavailable
	}
	properties := map[string]dbus.Variant{
		secretItem + "Label":      dbus.MakeVariant("Callum 加密主密钥"),
		secretItem + "Attributes": dbus.MakeVariant(map[string]string{"service": service, "account": name}),
	}
	var item, prompt dbus.ObjectPath
	if err := conn.Object(secretDestination, collection).CallWithContext(ctx, "org.freedesktop.Secret.Collection.CreateItem", 0, properties, secret{session, []byte{}, key, "application/octet-stream"}, true).Store(&item, &prompt); err != nil || prompt != "/" || item == "/" {
		return credentials.ErrUnavailable
	}
	return ctx.Err()
}
