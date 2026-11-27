//go:build darwin || linux

package processgroup

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestStopConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		exited, darwin bool
		kill           error
		probes         []error
		want           error
	}{
		{name: "absent", kill: syscall.ESRCH},
		{name: "killed"},
		{name: "live_permission_denied", darwin: true, kill: syscall.EPERM, want: syscall.EPERM},
		{name: "linux_permission_denied", exited: true, kill: syscall.EPERM, want: syscall.EPERM},
		{name: "other_kill_error", exited: true, darwin: true, kill: syscall.EIO, want: syscall.EIO},
		{name: "exiting", exited: true, darwin: true, kill: syscall.EPERM, probes: []error{syscall.EPERM, nil, syscall.ESRCH}},
		{name: "still_exists", exited: true, darwin: true, kill: syscall.EPERM, probes: []error{nil}, want: context.DeadlineExceeded},
		{name: "permission_persists", exited: true, darwin: true, kill: syscall.EPERM, probes: []error{syscall.EPERM}, want: context.DeadlineExceeded},
		{name: "probe_error", exited: true, darwin: true, kill: syscall.EPERM, probes: []error{syscall.EIO}, want: syscall.EIO},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				kills, probes := 0, 0
				err := stop(tc.exited, tc.darwin, 100*time.Millisecond, func(signal syscall.Signal) error {
					if signal == syscall.SIGKILL {
						kills++
						return tc.kill
					}
					if signal != 0 || len(tc.probes) == 0 {
						t.Fatalf("不应发送额外信号 %v", signal)
					}
					value := tc.probes[min(probes, len(tc.probes)-1)]
					probes++
					return value
				})
				if !errors.Is(err, tc.want) || kills != 1 {
					t.Fatalf("停止结果 err=%v kills=%d", err, kills)
				}
				if len(tc.probes) > 0 && probes < len(tc.probes) {
					t.Fatalf("缺少消失确认: %d", probes)
				}
				if tc.want != nil && len(tc.probes) > 0 && !errors.Is(err, syscall.EPERM) {
					t.Fatalf("丢失原始 EPERM: %v", err)
				}
			})
		})
	}
}

func TestStopRejectsInvalidIdentity(t *testing.T) {
	for _, pid := range []int{-1, 0} {
		if !errors.Is(Stop(pid, true, time.Second), syscall.EINVAL) {
			t.Fatal("接受了无效进程组")
		}
	}
}
