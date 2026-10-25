package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/deploymenttheory/go-ios-vm/internal/preflight"
)

// preflightProbes reads the host; tests replace it.
var preflightProbes = preflight.DefaultProbes

func preflightFlags(fs *flag.FlagSet) {
	fs.Bool("json", false, "print the report as JSON")
}

func runPreflight(ctx context.Context, inv *invocation) error {
	if inv.library == "" || inv.ipswCache == "" {
		return usagef("host preflight: --library and --ipsw-cache need values")
	}
	report := preflight.Run(ctx, preflightProbes(inv.env.GOOS, inv.env.GOARCH), preflight.Paths{
		IPSWCache: inv.ipswCache, Library: inv.library, Scratch: os.TempDir(),
	})
	var out string
	if inv.flag("json") {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("host preflight: %w", err)
		}
		out = string(data) + "\n"
	} else {
		out = formatPreflight(report)
	}
	if err := write(inv.env.Stdout, "preflight report", out); err != nil {
		return err
	}
	if err := report.RequireCreate(); err != nil {
		return fmt.Errorf("host preflight: %w", err)
	}
	return nil
}

func formatPreflight(r preflight.Report) string {
	var b strings.Builder
	b.WriteString("Host preflight (read-only; not evidence that a machine restores or boots)\n\n")
	rows := make([][]string, 0, 1+len(r.Results))
	rows = append(rows, []string{"STATUS", "CHECK", "OBSERVED"})
	counts := map[preflight.Status]int{}
	for _, res := range r.Results {
		counts[res.Status]++
		title := res.Title
		if res.Blocking {
			title += " *"
		}
		rows = append(rows, []string{string(res.Status), title, res.Observed})
	}
	writeTable(&b, "", 2, rows)
	b.WriteString("* blocks machine creation when unsupported\n")

	var remedies [][]string
	for _, res := range r.Results {
		if res.Remedy != "" && res.Status != preflight.Supported {
			remedies = append(remedies, []string{res.Title + ":", res.Remedy})
		}
	}
	if len(remedies) > 0 {
		b.WriteString("\nManual preparation (this runner never changes host policy):\n")
		writeTable(&b, "  ", 1, remedies)
	}

	b.WriteString("\nStorage:\n")
	for _, v := range r.Volumes {
		fmt.Fprintf(&b, "  %s: %s free, %s needed\n", v.MountPath, preflight.Size(v.Free), preflight.Size(v.Need()))
		for _, p := range v.Paths {
			fmt.Fprintf(&b, "    %s\n", p)
		}
		demand := make([][]string, 0, len(v.Demand))
		for _, d := range v.Demand {
			demand = append(demand, []string{"- " + d.What, preflight.Size(d.Bytes), "(" + d.Basis + ")"})
		}
		writeTable(&b, "    ", 2, demand)
	}
	if len(r.Cache) == 0 {
		b.WriteString("  IPSW cache: empty\n")
	} else {
		names := make([]string, len(r.Cache))
		for i, e := range r.Cache {
			names[i] = e.Name + " (" + preflight.Size(e.Size) + ")"
		}
		fmt.Fprintf(&b, "  IPSW cache: %s; existing entries are never modified\n", strings.Join(names, ", "))
	}

	b.WriteString("\nNotes:\n")
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "  %s\n", n)
	}
	fmt.Fprintf(&b, "\n%d supported, %d unsupported (%d blocking), %d unverified\n",
		counts[preflight.Supported], counts[preflight.Unsupported], len(r.Unmet()), counts[preflight.Unverified])
	return b.String()
}
