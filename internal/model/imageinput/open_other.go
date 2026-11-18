//go:build !unix

package imageinput

import "os"

const readFlags = os.O_RDONLY
