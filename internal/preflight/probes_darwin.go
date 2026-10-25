package preflight

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"howett.net/plist"

	"github.com/deploymenttheory/go-ios-vm/internal/host/native"
)

// DefaultProbes reads the real host. Every probe only reads.
func DefaultProbes(goos, goarch string) Probes {
	return Probes{
		GOOS: goos, GOARCH: goarch,
		ProductVersion: func() (string, error) {
			v, err := syscall.Sysctl("kern.osproductversion")
			if err != nil {
				return "", fmt.Errorf("read macOS version: %w", err)
			}
			return v, nil
		},
		Sysctl: func(name string) (int64, error) {
			out, err := run(context.Background(), "/usr/sbin/sysctl", "-n", name)
			if err != nil {
				return 0, err
			}
			n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("sysctl %s: %w", name, err)
			}
			return n, nil
		},
		Capabilities: func() ([]string, error) {
			r, err := native.Inventory()
			if err != nil {
				return nil, fmt.Errorf("inventory Virtualization interfaces: %w", err)
			}
			var missing []string
			for _, m := range r.RequiredMissing() {
				missing = append(missing, m.Capability.String())
			}
			return missing, nil
		},
		ResearchModel: func() (bool, error) {
			ok, err := native.HardwareModelSupported(native.ResearchHardwareModel)
			if err != nil {
				return false, fmt.Errorf("research hardware model: %w", err)
			}
			return ok, nil
		},
		Entitlements: entitlements,
		Command:      run,
		LookPath: func(name string) (string, error) {
			path, err := exec.LookPath(name)
			if err != nil {
				return "", fmt.Errorf("find %s: %w", name, err)
			}
			return path, nil
		},
		Space:        space,
		CacheEntries: cacheEntries,
	}
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s: %s", filepath.Base(name), msg) //nolint:err113 // the tool's own message
	}
	return stdout.String(), nil
}

// entitlements reads the running executable's code-signing entitlements.
func entitlements(ctx context.Context) (map[string]bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find executable: %w", err)
	}
	out, err := run(ctx, "/usr/bin/codesign", "-d", "--entitlements", "-", "--xml", exe)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	if strings.TrimSpace(out) == "" {
		return have, nil
	}
	var values map[string]any
	if _, err := plist.Unmarshal([]byte(out), &values); err != nil {
		return nil, fmt.Errorf("parse entitlements: %w", err)
	}
	for k, v := range values {
		if b, ok := v.(bool); ok && b {
			have[k] = true
		}
	}
	return have, nil
}

// space reports the file system holding path. A path that does not exist yet
// is measured at its nearest existing parent, where it would be created.
func space(path string) (Space, error) {
	p := filepath.Clean(path)
	for {
		if _, err := os.Stat(p); err == nil {
			break
		}
		parent := filepath.Dir(p)
		if parent == p {
			return Space{}, fmt.Errorf("no existing parent of %s", path) //nolint:err113 // describes the path
		}
		p = parent
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return Space{}, fmt.Errorf("statfs %s: %w", p, err)
	}
	mount := string(bytes.TrimRight(int8sToBytes(st.Mntonname[:]), "\x00"))
	return Space{
		ID:        fmt.Sprintf("%x-%x", st.Fsid.Val[0], st.Fsid.Val[1]),
		MountPath: mount,
		Free:      int64(st.Bavail) * int64(st.Bsize), //nolint:gosec // block counts fit in int64
	}, nil
}

func int8sToBytes(s []int8) []byte {
	b := make([]byte, len(s))
	for i, c := range s {
		b[i] = byte(c) //nolint:gosec // reinterprets C char bytes
	}
	return b
}

// cacheEntries lists regular files in the IPSW cache, ignoring hidden ones.
func cacheEntries(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read IPSW cache: %w", err)
	}
	var entries []Entry
	for _, d := range des {
		if strings.HasPrefix(d.Name(), ".") || !d.Type().IsRegular() {
			continue
		}
		info, err := d.Info()
		if err != nil {
			return nil, fmt.Errorf("read IPSW cache: %w", err)
		}
		entries = append(entries, Entry{Name: d.Name(), Size: info.Size()})
	}
	return entries, nil
}
