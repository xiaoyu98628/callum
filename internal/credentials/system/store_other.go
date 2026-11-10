//go:build !darwin && !windows && !linux

package system

import (
	"context"
	"github.com/xiaoyu98628/callum/internal/credentials"
)

func (Store) Load(context.Context, string) ([]byte, error) { return nil, credentials.ErrUnavailable }
func (Store) Save(context.Context, string, []byte) error   { return credentials.ErrUnavailable }
