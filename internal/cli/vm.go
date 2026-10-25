package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/host"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
)

func (inv *invocation) libraryStore() (machine.Library, error) {
	if inv.library == "" {
		return machine.Library{}, usagef("no machine library: pass --library")
	}
	return machine.Library{Dir: inv.library}, nil
}

func runList(_ context.Context, inv *invocation) error {
	lib, err := inv.libraryStore()
	if err != nil {
		return err
	}
	entries, err := lib.List()
	if err != nil {
		return fmt.Errorf("vm list: %w", err)
	}
	var b strings.Builder
	if len(entries) == 0 {
		fmt.Fprintf(&b, "No machines in %s.\n", lib.Dir)
		return write(inv.env.Stdout, "machine list", b.String())
	}
	rows := [][]string{{"NAME", "STAGES", "NATIVE (LAST OBSERVED)", "PROBLEM"}}
	for _, e := range entries {
		if e.Err != nil {
			rows = append(rows, []string{e.Name, "-", "-", e.Err.Error()})
			continue
		}
		stages, problem := stageSummary(e.Machine)
		rows = append(rows, []string{e.Name, stages, nativeSummary(e.Machine), problem})
	}
	writeTable(&b, "", 2, rows)
	return write(inv.env.Stdout, "machine list", b.String())
}

func stageSummary(m *machine.Machine) (summary, problem string) {
	states, err := m.Stages()
	if err != nil {
		return "-", err.Error()
	}
	done := 0
	for _, s := range states {
		switch {
		case s.Completed:
			done++
		case s.Problem != nil && problem == "":
			problem = string(s.Stage) + ": " + s.Problem.Error()
		}
	}
	return strconv.Itoa(done) + "/" + strconv.Itoa(len(states)), problem
}

func nativeSummary(m *machine.Machine) string {
	state, at, err := m.LastNative()
	switch {
	case err != nil:
		return err.Error()
	case state == host.StateUnknown:
		return "unknown"
	default:
		return string(state) + " at " + at.Format(time.RFC3339)
	}
}

func runInfo(_ context.Context, inv *invocation) error {
	lib, err := inv.libraryStore()
	if err != nil {
		return err
	}
	m, err := lib.Open(inv.args[0])
	if err != nil {
		return fmt.Errorf("vm info: %w", err)
	}
	states, err := m.Stages()
	if err != nil {
		return fmt.Errorf("vm info: %w", err)
	}
	c := m.Config
	fingerprint := sha256.Sum256(c.Identifier)
	native := "unknown (never observed)"
	if state, at, err := m.LastNative(); err != nil {
		native = err.Error()
	} else if state != host.StateUnknown {
		native = fmt.Sprintf("%s, last observed %s (may be stale)", state, at.Format(time.RFC3339))
	}
	storage := make([]string, 0, 5)
	for _, name := range []string{c.Files.Disk, c.Files.NVRAM, c.Files.SEPStorage, c.Files.ROM, c.Files.SEPROM} {
		storage = append(storage, name+" ("+presence(filepath.Join(m.Dir, name))+")")
	}

	var b strings.Builder
	writeTable(&b, "", 1, [][]string{
		{"Name:", m.Name},
		{"Directory:", m.Dir},
		{"Identifier:", fmt.Sprintf("%d bytes, SHA-256 %s", len(c.Identifier), hex.EncodeToString(fingerprint[:8]))},
		{"Created:", c.CreatedAt.Format(time.RFC3339)},
		{"Firmware:", c.Pair.IPhone.String() + " + " + c.Pair.CloudOS.String()},
		{"Hardware:", fmt.Sprintf("%d CPUs, %d MiB memory, %d GB sparse disk, %s network",
			c.Hardware.CPUs, c.Hardware.MemoryMiB, c.Hardware.DiskSizeGB, c.Hardware.Network)},
		{"Storage:", strings.Join(storage, ", ")},
		{"Native:", native},
		{"Guest:", "unknown (no daemon response is recorded)"},
	})
	b.WriteString("Stages:\n")
	rows := make([][]string, 0, len(states))
	for _, s := range states {
		rows = append(rows, []string{string(s.Stage), describeStage(s)})
	}
	writeTable(&b, "  ", 2, rows)
	return write(inv.env.Stdout, "machine info", b.String())
}

func describeStage(s machine.StageState) string {
	switch {
	case s.Completed:
		return fmt.Sprintf("completed (%d evidence file(s) verified)", len(s.Receipt.Evidence))
	case s.Problem != nil:
		return "not completed: " + s.Problem.Error()
	case s.Latest == nil:
		return "pending"
	case s.Latest.Status == machine.StatusRunning:
		return "running, or interrupted if no session owns the machine"
	default:
		return string(s.Latest.Status) + ": " + s.Latest.Error
	}
}

func presence(path string) string {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "absent"
	case err != nil:
		return err.Error()
	default:
		return strconv.FormatInt(info.Size(), 10) + " bytes"
	}
}
