//go:build !darwin && !linux && !windows

package run

import (
	"github.com/xiaoyu98628/callum/internal/tool"
	"os"
)

type process struct{}

func (*process) processID() int { return 0 }

func openTargetPath(string) (*os.File, error) { return nil, tool.ErrUnavailable }

func startProcess(*fixedCommand, *os.File, *os.File, *os.File) (*process, error) {
	return nil, tool.ErrUnavailable
}
func (*process) poll() (processExit, bool, error)        { return processExit{}, false, tool.ErrUnavailable }
func (*process) stop() error                             { return tool.ErrUnavailable }
func (*process) close() error                            { return nil }
func fileIdentity(*os.File, os.FileInfo) (string, error) { return "", tool.ErrUnavailable }
