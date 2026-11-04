//go:build !unix

package chatcompletions

import "os"

func openImageFile(path string) (*os.File, error) { return os.Open(path) }
