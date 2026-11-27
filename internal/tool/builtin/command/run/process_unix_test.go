//go:build darwin || linux

package run

import "testing"

func TestUnixProcessGroupCleanup(t *testing.T) { testProcessTree(t) }
