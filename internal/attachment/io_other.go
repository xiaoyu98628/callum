//go:build !unix

package attachment

import "os"

const readFlags = os.O_RDONLY
