//go:build !windows

package main

import "errors"

func relaxACL(string) error { return errors.New("ACL 绕过测试仅适用于 Windows") }
