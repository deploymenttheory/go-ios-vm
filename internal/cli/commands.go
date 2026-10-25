package cli

import (
	"context"
	"flag"
)

// command is one leaf of the command tree.
type command struct {
	// path is the words that select the command, for example {"vm", "create"}.
	path []string
	// args documents the positional arguments, for example "<name>".
	args string
	// nargs is the exact number of positional arguments.
	nargs   int
	summary string
	// ticket names the backlog issue that delivers an unimplemented command.
	ticket string
	// native marks commands that need the darwin/arm64 host.
	native bool
	// flags registers command-specific flags.
	flags func(fs *flag.FlagSet)
	// run is nil until the command is implemented.
	run func(ctx context.Context, inv *invocation) error
}

func (c *command) implemented() bool { return c.run != nil }

// commands is the agreed command tree, in help order.
var commands = []*command{
	{
		path: []string{"host", "preflight"}, summary: "Check host prerequisites without changing host policy",
		native: true, flags: preflightFlags, run: runPreflight,
	},
	{
		path: []string{"vm", "create"}, args: "<name>", nargs: 1,
		summary: "Create, restore and validate a new machine",
		ticket:  "E2E-01 (#96)", native: true, flags: createFlags,
	},
	{
		path: []string{"vm", "launch"}, args: "<name>", nargs: 1, summary: "Start a machine in an interactive window",
		ticket: "E2E-01 (#96)", native: true,
	},
	{
		path: []string{"vm", "stop"}, args: "<name>", nargs: 1, summary: "Stop a machine owned by another session",
		ticket: "HOST-04 (#79)", native: true,
	},
	{
		path: []string{"vm", "list"}, summary: "List machines in the library",
		native: true, run: runList,
	},
	{
		path: []string{"vm", "info"}, args: "<name>", nargs: 1, summary: "Show a machine's identity, stages and state",
		native: true, run: runInfo,
	},
	{
		path: []string{"fw", "prepare"}, args: "<name>", nargs: 1, summary: "Acquire and prepare the firmware pair for a machine",
		ticket: "FW-06 (#90)", native: true,
	},
	{
		path: []string{"fw", "patch"}, args: "<name>", nargs: 1, summary: "Apply the firmware patches and record receipts",
		ticket: "FW-04 (#88)", native: true,
	},
	{
		path: []string{"restore"}, args: "<name>", nargs: 1, summary: "Restore prepared firmware onto a machine in DFU",
		ticket: "REST-05 (#87)", native: true,
	},
	{
		path: []string{"version"}, summary: "Print the version, source pins and supported firmware pair",
		run: runVersion,
	},
}

// createDefaults are the agreed machine defaults, subject to native validation.
const (
	defaultCPUs     = 8
	defaultMemory   = 8192 // MiB
	defaultDiskSize = 64   // GB, sparse
	defaultNetwork  = "nat"
)

func createFlags(fs *flag.FlagSet) {
	fs.Uint("cpus", defaultCPUs, "virtual CPU count")
	fs.Uint64("memory", defaultMemory, "memory size in MiB")
	fs.Uint64("disk-size", defaultDiskSize, "sparse disk size in GB")
	fs.String("network", defaultNetwork, "network mode")
}
