// Package cli implements the go-ios-vm command line. Requested results and
// progress go to stdout; diagnostics go to stderr.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/deploymenttheory/go-ios-vm/internal/buildinfo"
	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
)

const name = "go-ios-vm"

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1
	// ExitUsage reports invalid arguments or flags.
	ExitUsage = 2
	// ExitUnsupported reports an unsupported platform or an operation this
	// build does not implement.
	ExitUnsupported = 3
)

// Errors that end a command with ExitUnsupported. Both match
// errors.ErrUnsupported.
var (
	ErrUnsupportedPlatform error = unsupported("unsupported platform")
	ErrNotImplemented      error = unsupported("not implemented in this build")
)

type unsupported string

func (e unsupported) Error() string { return string(e) }

func (unsupported) Is(target error) bool { return target == errors.ErrUnsupported }

// Env is the process environment a command runs in.
type Env struct {
	Stdout, Stderr io.Writer
	GOOS, GOARCH   string
	UserCacheDir   func() (string, error)
	UserConfigDir  func() (string, error)
}

// ProcessEnv returns the environment of the current process.
func ProcessEnv() Env {
	return Env{
		Stdout: os.Stdout, Stderr: os.Stderr,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		UserCacheDir: os.UserCacheDir, UserConfigDir: os.UserConfigDir,
	}
}

// usageError reports invalid command-line input.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error { return &usageError{fmt.Sprintf(format, args...)} }

// invocation is one parsed command.
type invocation struct {
	env       Env
	cmd       *command
	library   string
	ipswCache string
	args      []string
}

// Run runs the command line args and returns the process exit code.
func Run(ctx context.Context, args []string, env Env) int {
	err := run(ctx, args, env)
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, new(*usageError)):
		diagnose(env.Stderr, "%s: %v\nRun '%s help' for usage.\n", name, err, name)
		return ExitUsage
	case errors.Is(err, errors.ErrUnsupported):
		diagnose(env.Stderr, "%s: %v\n", name, err)
		return ExitUnsupported
	default:
		diagnose(env.Stderr, "%s: %v\n", name, err)
		return ExitFailure
	}
}

// diagnose writes a diagnostic. A failed write to stderr cannot be reported.
func diagnose(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func run(ctx context.Context, args []string, env Env) error {
	inv := &invocation{env: env}
	defaults := defaultDirs(env)

	root := newFlagSet(name)
	inv.bindGlobal(root, defaults)
	if err := root.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeRootHelp(env.Stdout, defaults)
		}
		return usagef("%v", err)
	}
	words := root.Args()
	if len(words) == 0 {
		return usagef("no command given")
	}
	if words[0] == "help" {
		return help(env.Stdout, words[1:], defaults)
	}

	cmd, rest, err := lookup(words)
	if err != nil {
		return err
	}
	inv.cmd = cmd

	fs := newFlagSet(name + " " + cmd.name())
	inv.bindGlobal(fs, defaults)
	if cmd.flags != nil {
		cmd.flags(fs)
	}
	inv.args, err = parseInterspersed(fs, rest)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeCommandHelp(env.Stdout, cmd, defaults)
		}
		return usagef("%s: %v", cmd.name(), err)
	}
	if len(inv.args) != cmd.nargs {
		return usagef("%s: want %d argument(s) %s, got %d", cmd.name(), cmd.nargs, cmd.args, len(inv.args))
	}

	if cmd.native && (env.GOOS != "darwin" || env.GOARCH != "arm64") {
		return fmt.Errorf("%s: %w %s/%s; %s requires darwin/arm64 (Apple Silicon, macOS 27 or later)",
			cmd.name(), ErrUnsupportedPlatform, env.GOOS, env.GOARCH, name)
	}
	if !cmd.implemented() {
		return fmt.Errorf("%s: %w (tracked by %s)", cmd.name(), ErrNotImplemented, cmd.ticket)
	}
	return cmd.run(ctx, inv)
}

func (c *command) name() string { return strings.Join(c.path, " ") }

// lookup selects the command named by the leading words.
func lookup(words []string) (*command, []string, error) {
	for _, c := range commands {
		if len(words) >= len(c.path) && slices.Equal(words[:len(c.path)], c.path) {
			return c, words[len(c.path):], nil
		}
	}
	var group []string
	for _, c := range commands {
		if len(c.path) > 1 && c.path[0] == words[0] {
			group = append(group, c.path[1])
		}
	}
	if len(group) > 0 {
		if len(words) == 1 {
			return nil, nil, usagef("%s: missing subcommand (one of: %s)", words[0], strings.Join(group, ", "))
		}
		return nil, nil, usagef("%s: unknown subcommand %q (one of: %s)", words[0], words[1], strings.Join(group, ", "))
	}
	return nil, nil, usagef("unknown command %q", words[0])
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// dirs are the default library and cache directories, or why they are unknown.
type dirs struct {
	library, ipswCache       string
	libraryErr, ipswCacheErr error
}

func defaultDirs(env Env) dirs {
	var d dirs
	if config, err := env.UserConfigDir(); err != nil {
		d.libraryErr = err
	} else {
		d.library = machine.DefaultLibraryDir(config)
	}
	if cache, err := env.UserCacheDir(); err != nil {
		d.ipswCacheErr = err
	} else {
		d.ipswCache = firmware.DefaultCacheDir(cache)
	}
	return d
}

// bindGlobal registers the global flags on fs. Binding the same fields on the
// root and leaf flag sets lets them appear before or after the command.
func (inv *invocation) bindGlobal(fs *flag.FlagSet, d dirs) {
	if inv.library == "" {
		inv.library = d.library
	}
	if inv.ipswCache == "" {
		inv.ipswCache = d.ipswCache
	}
	fs.StringVar(&inv.library, "library", inv.library, "machine library `dir`")
	fs.StringVar(&inv.ipswCache, "ipsw-cache", inv.ipswCache, "IPSW cache `dir`")
}

// parseInterspersed parses flags that appear before, between or after
// positional arguments. Arguments after "--" are always positional.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var tail []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, tail = args[:i], args[i+1:]
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
	return append(positional, tail...), nil
}

func help(w io.Writer, words []string, d dirs) error {
	if len(words) == 0 {
		return writeRootHelp(w, d)
	}
	cmd, rest, err := lookup(words)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("help: unknown command %q", strings.Join(words, " "))
	}
	return writeCommandHelp(w, cmd, d)
}

func writeRootHelp(w io.Writer, d dirs) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s creates, restores and runs an iOS research virtual machine on an Apple Silicon Mac.\n\n", name)
	fmt.Fprintf(&b, "Usage:\n  %s [global flags] <command> [flags] [arguments]\n  %s help [command]\n\nCommands:\n", name, name)
	var rows [][]string
	for _, c := range commands {
		status := ""
		if !c.implemented() {
			status = "not implemented: " + c.ticket
		}
		rows = append(rows, []string{strings.TrimSpace(c.name() + " " + c.args), c.summary, status})
	}
	writeTable(&b, "  ", 2, rows)
	b.WriteString("\nGlobal flags, accepted before or after the command:\n")
	writeTable(&b, "  ", 2, [][]string{
		{"--library dir", "machine library (" + describeDefault(d.library, d.libraryErr) + ")"},
		{"--ipsw-cache dir", "IPSW cache (" + describeDefault(d.ipswCache, d.ipswCacheErr) + ")"},
	})
	fmt.Fprintf(&b, "\nSupported firmware pair: %s + %s\n", firmware.SupportedPair.IPhone, firmware.SupportedPair.CloudOS)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeTable writes rows as left-aligned columns separated by at least gap
// spaces.
func writeTable(b *strings.Builder, indent string, gap int, rows [][]string) {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	for _, row := range rows {
		var line strings.Builder
		line.WriteString(indent)
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+gap))
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteString("\n")
	}
}

func describeDefault(dir string, err error) string {
	if err != nil {
		return "no default: " + err.Error()
	}
	return fmt.Sprintf("default %q", dir)
}

func writeCommandHelp(w io.Writer, c *command, d dirs) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s %s [flags] %s\n\n%s.\n", name, c.name(), c.args, c.summary)
	if !c.implemented() {
		fmt.Fprintf(&b, "\nStatus: not implemented in this build; tracked by %s.\n", c.ticket)
	}
	fs := newFlagSet(name + " " + c.name())
	(&invocation{}).bindGlobal(fs, d)
	if c.flags != nil {
		c.flags(fs)
	}
	b.WriteString("\nFlags:\n")
	fs.SetOutput(&b)
	fs.PrintDefaults()
	_, err := io.WriteString(w, b.String())
	return err
}

func runVersion(_ context.Context, inv *invocation) error {
	info := buildinfo.Read()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", name, info.Version)
	var rows [][]string
	if info.Revision != "" {
		revision := info.Revision
		if info.Modified {
			revision += " (modified)"
		}
		rows = append(rows, []string{"revision:", revision})
	}
	if info.GoVersion != "" {
		rows = append(rows, []string{"go:", info.GoVersion + " " + inv.env.GOOS + "/" + inv.env.GOARCH})
	}
	rows = append(rows,
		[]string{"vphone-cli:", buildinfo.VPhoneCLIRevision},
		[]string{"go-bindings-macosplatform:", buildinfo.MacOSBindingsRevision},
		[]string{"firmware pair:", firmware.SupportedPair.IPhone.String() + " + " + firmware.SupportedPair.CloudOS.String()},
	)
	writeTable(&b, "", 1, rows)
	_, err := io.WriteString(inv.env.Stdout, b.String())
	return err
}
