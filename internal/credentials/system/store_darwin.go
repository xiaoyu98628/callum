//go:build darwin

package system

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"strings"

	"github.com/xiaoyu98628/callum/internal/credentials"
)

// 主密钥通过 stdin 交给 security 的交互解析器，不出现在命令参数或环境中。
func (Store) Save(ctx context.Context, name string, key []byte) error {
	if !validName(name) || len(key) != 32 {
		return credentials.ErrInvalid
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "-i")
	command.Stdin = strings.NewReader("add-generic-password -U -s " + service + " -a " + name + " -w " + base64.StdEncoding.EncodeToString(key) + "\n")
	if err := command.Run(); err != nil {
		return errors.Join(credentials.ErrUnavailable, ctx.Err())
	}
	return nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, credentials.ErrInvalid
	}
	return b.Buffer.Write(p)
}

func (Store) Load(ctx context.Context, name string) ([]byte, error) {
	if !validName(name) {
		return nil, credentials.ErrInvalid
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-a", name, "-w")
	var output boundedOutput
	command.Stdout = &output
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 44 {
			return nil, credentials.ErrNotFound
		}
		return nil, credentials.ErrUnavailable
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(output.String()))
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, credentials.ErrInvalid
	}
	return key, nil
}
