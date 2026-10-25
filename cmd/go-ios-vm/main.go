// Command go-ios-vm creates, restores and runs an iOS research virtual machine
// on an Apple Silicon Mac.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/deploymenttheory/go-ios-vm/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], cli.ProcessEnv())
	stop()
	os.Exit(code)
}
