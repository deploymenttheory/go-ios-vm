package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/viper"
)

type countedWriter struct {
	writer io.Writer
	bytes  int
}

func (w *countedWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.bytes += n
	return n, err
}

// requestedFormat handles errors that Cobra returns before PersistentPreRunE.
func requestedFormat(args []string) string {
	format, configPath := "", ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--format" && i+1 < len(args):
			format = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--format="):
			format = strings.TrimPrefix(args[i], "--format=")
		case args[i] == "--config" && i+1 < len(args):
			configPath = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		}
	}
	if format != "" {
		return format
	}
	if format = os.Getenv("GOIOSVM_FORMAT"); format != "" {
		return format
	}
	if configPath != "" {
		v := viper.New()
		v.SetConfigFile(configPath)
		if v.ReadInConfig() == nil {
			return v.GetString("format")
		}
	}
	return "text"
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	format := requestedFormat(args)
	output := &countedWriter{writer: stdout}
	root := newCommand(output, stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	return finishCLI(ctx, format, output, stderr, err)
}

func finishCLI(ctx context.Context, format string, output *countedWriter, stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	code := 1
	var partial *partialError
	if errors.As(err, &partial) {
		code = 2
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = 130
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}
	if code == 2 && output.bytes > 0 {
		return code
	}
	if format == "json" && output.bytes == 0 {
		_ = writeDiagnostic(output, format, err)
	} else {
		// A failed writer may have emitted part of a result. Never append another JSON object.
		_ = writeDiagnostic(stderr, "text", err)
	}
	return code
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if code := runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}
