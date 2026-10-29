package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	_, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Print("hello world")
}
