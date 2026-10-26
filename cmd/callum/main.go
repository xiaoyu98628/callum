package main

import (
	"context"
	"os"

	"github.com/xiaoyu98628/callum/internal/bootstrap"
)

func main() {
	if err := bootstrap.Run(context.Background(), os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
