package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	root := newCommand(os.Stdout, os.Stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		code := 1
		var partial *partialError
		if errors.As(err, &partial) {
			code = 2
		} else {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		if ctx.Err() != nil {
			code = 130
		}
		os.Exit(code)
	}
}
