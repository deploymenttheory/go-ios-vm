package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const lockFile = "lock"

// ErrBusy reports a machine owned by another process.
var ErrBusy = errors.New("machine is owned by another process")

// ErrNotOwner reports a mutation without the machine's current owner.
var ErrNotOwner = errors.New("not the machine's current owner")

// Holder describes the process that owns a machine.
type Holder struct {
	PID        int       `json:"pid"`
	Purpose    string    `json:"purpose"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// Owner is exclusive ownership of one machine, held through an OS lock on the
// machine's lock file. Ownership ends with Release or when the process exits.
type Owner struct {
	m *Machine
	f *os.File
}

// Lock takes exclusive ownership of m without waiting. If another process owns
// m, the error wraps ErrBusy and names that process.
func (m *Machine) Lock(purpose string) (*Owner, error) {
	path := filepath.Join(m.Dir, lockFile)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // the machine's own lock file
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", m.Name, err)
	}
	if err := lockExclusive(f); err != nil {
		holder := readHolder(f)
		_ = f.Close()
		if errors.Is(err, errWouldBlock) {
			return nil, fmt.Errorf("%w: %s is held by %s", ErrBusy, m.Name, holder)
		}
		return nil, fmt.Errorf("lock %s: %w", m.Name, err)
	}
	o := &Owner{m: m, f: f}
	h := Holder{PID: os.Getpid(), Purpose: purpose, AcquiredAt: time.Now().UTC()}
	data, err := json.Marshal(h)
	if err == nil {
		err = f.Truncate(0)
	}
	if err == nil {
		_, err = f.WriteAt(append(data, '\n'), 0)
	}
	if err != nil {
		o.Release()
		return nil, fmt.Errorf("record owner of %s: %w", m.Name, err)
	}
	if err := removeTemps(m.Dir); err != nil {
		o.Release()
		return nil, err
	}
	return o, nil
}

func readHolder(f *os.File) string {
	data, err := io.ReadAll(io.NewSectionReader(f, 0, 4096))
	var h Holder
	if err != nil || json.Unmarshal(data, &h) != nil || h.PID == 0 {
		return "another process"
	}
	return fmt.Sprintf("pid %d (%s) since %s", h.PID, h.Purpose, h.AcquiredAt.Format(time.RFC3339))
}

// Release ends ownership. It is safe to call more than once.
func (o *Owner) Release() {
	if o == nil || o.f == nil {
		return
	}
	_ = o.f.Close() // closing the descriptor releases the lock
	o.f = nil
}

// owns reports whether o currently owns m.
func (o *Owner) owns(m *Machine) error {
	if o == nil || o.f == nil || o.m == nil || o.m.Dir != m.Dir {
		return fmt.Errorf("%w: %s", ErrNotOwner, m.Name)
	}
	return nil
}

// Check reports whether o currently owns m, so packages outside machine can
// require ownership before changing a machine's files.
func (o *Owner) Check(m *Machine) error { return o.owns(m) }
