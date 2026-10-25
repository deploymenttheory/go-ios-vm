package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tempPrefix marks files an atomic write has not yet published. Readers ignore
// them and owners remove leftovers from interrupted writes.
const tempPrefix = ".tmp-"

// afterTempWrite runs between writing a temporary file and publishing it. Tests
// set it to interrupt a write at that point.
var afterTempWrite = func() {}

// writeFileAtomic replaces path with data: the data is written and synced to a
// temporary file in the same directory, renamed over path, and the directory
// is synced. A reader sees either the old content or the new.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, tempPrefix+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp := f.Name()
	published := false
	defer func() {
		if !published {
			_ = os.Remove(tmp) //nolint:gosec // temporary file this write created
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	afterTempWrite()
	if err := os.Rename(tmp, path); err != nil { //nolint:gosec // paths inside a validated machine directory
		return fmt.Errorf("publish %s: %w", path, err)
	}
	published = true
	return syncDir(dir)
}

func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // the machine's own directory
	if err != nil {
		return fmt.Errorf("sync %s: %w", dir, err)
	}
	defer d.Close() //nolint:errcheck // read-only
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dir, err)
	}
	return nil
}

// removeTemps deletes leftovers of interrupted atomic writes in dir.
func removeTemps(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tempPrefix) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil { //nolint:gosec // an interrupted write in the owned directory
				return fmt.Errorf("remove interrupted write: %w", err)
			}
		}
	}
	return nil
}
