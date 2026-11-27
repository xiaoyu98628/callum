//go:build darwin || linux

package run

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestUnixProcessGroupCleanup(t *testing.T) { testProcessTree(t) }

func TestWaitForProcessGroupRemoval(t *testing.T) {
	for _, tc := range []struct {
		name    string
		probes  []error
		wantErr error
	}{
		{"absent", []error{syscall.ESRCH}, nil},
		{"exiting", []error{syscall.EPERM, syscall.EPERM, syscall.ESRCH}, nil},
		{"live_then_absent", []error{nil, syscall.ESRCH}, nil},
		{"permission_denied", []error{syscall.EPERM}, context.DeadlineExceeded},
		{"still_live", []error{nil}, context.DeadlineExceeded},
		{"probe_failure", []error{syscall.EIO}, syscall.EIO},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := waitForProcessGroupRemoval(func() error {
				value := tc.probes[min(calls, len(tc.probes)-1)]
				calls++
				return value
			}, 100*time.Millisecond)
			if !errors.Is(err, tc.wantErr) || calls < len(tc.probes) {
				t.Fatalf("进程组消失确认不符: err=%v calls=%d", err, calls)
			}
		})
	}
}
