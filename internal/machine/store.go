package machine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
	"github.com/deploymenttheory/go-ios-vm/internal/guest"
	"github.com/deploymenttheory/go-ios-vm/internal/host"
)

// SchemaVersion is the machine.json layout this build reads and writes.
const SchemaVersion = 1

// Files and directories inside a machine directory.
const (
	configFile  = "machine.json"
	nativeFile  = "native.json"
	stagesDir   = "stages"
	attemptsDir = "attempts"
)

var reservedNames = []string{configFile, nativeFile, stagesDir, attemptsDir, lockFile}

// Errors returned by the store.
var (
	ErrInvalidName       = errors.New("invalid machine name")
	ErrExists            = errors.New("machine already exists")
	ErrNotFound          = errors.New("machine not found")
	ErrCorrupt           = errors.New("machine record is corrupt")
	ErrUnsupportedSchema = errors.New("unsupported machine schema")
	ErrInvalidConfig     = errors.New("invalid machine configuration")
	ErrArtifactMissing   = errors.New("stage evidence is missing")
	ErrArtifactChanged   = errors.New("stage evidence has changed")
)

// Hardware is the virtual hardware a machine is created with.
type Hardware struct {
	CPUs       uint   `json:"cpus"`
	MemoryMiB  uint64 `json:"memory_mib"`
	DiskSizeGB uint64 `json:"disk_size_gb"`
	Network    string `json:"network"`
}

// Files names the storage that belongs to one machine. Each is a single file
// name inside the machine directory, so disk, NVRAM and SEP storage always
// travel with their identity.
type Files struct {
	Disk       string `json:"disk"`
	NVRAM      string `json:"nvram"`
	SEPStorage string `json:"sep_storage"`
	ROM        string `json:"rom"`
	SEPROM     string `json:"sep_rom"`
}

// DefaultFiles are the file names a new machine uses.
var DefaultFiles = Files{
	Disk: "disk.img", NVRAM: "nvram.bin", SEPStorage: "sep-storage.bin",
	ROM: "rom.bin", SEPROM: "sep-rom.bin",
}

// Config is a machine's durable configuration.
type Config struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	// Identifier is the native machine identifier created with the machine.
	// It is never regenerated: a machine whose record is lost or corrupt is
	// reported, not given a replacement identity.
	Identifier []byte        `json:"identifier"`
	CreatedAt  time.Time     `json:"created_at"`
	Pair       firmware.Pair `json:"pair"`
	Hardware   Hardware      `json:"hardware"`
	Files      Files         `json:"files"`
}

func (c *Config) validate() error {
	if err := validName(c.Name); err != nil {
		return err
	}
	if len(c.Identifier) == 0 {
		return fmt.Errorf("%w: %s has no machine identifier", ErrInvalidConfig, c.Name)
	}
	if c.Pair.IPhone.Build == "" || c.Pair.CloudOS.Build == "" {
		return fmt.Errorf("%w: %s has no firmware pair", ErrInvalidConfig, c.Name)
	}
	if c.Hardware.CPUs == 0 || c.Hardware.MemoryMiB == 0 || c.Hardware.DiskSizeGB == 0 || c.Hardware.Network == "" {
		return fmt.Errorf("%w: %s has incomplete hardware", ErrInvalidConfig, c.Name)
	}
	names := []string{c.Files.Disk, c.Files.NVRAM, c.Files.SEPStorage, c.Files.ROM, c.Files.SEPROM}
	for i, n := range names {
		if n == "" || n != filepath.Base(n) || n == "." || n == ".." || strings.HasPrefix(n, ".") ||
			slices.Contains(reservedNames, n) || slices.Contains(names[:i], n) {
			return fmt.Errorf("%w: %s: storage file %q must be a distinct plain file name inside the machine directory", ErrInvalidConfig, c.Name, n)
		}
	}
	return nil
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func validName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("%w %q: use 1-64 letters, digits, '.', '_' or '-', starting with a letter or digit", ErrInvalidName, name)
	}
	return nil
}

// Library is a directory of machines, one subdirectory each.
type Library struct {
	Dir string
}

// Machine is one machine in a library.
type Machine struct {
	Name   string
	Dir    string
	Config Config
}

// Create creates a machine from cfg and returns it owned by the caller. The
// name must be new; an existing machine is never replaced.
func (l Library) Create(cfg Config, purpose string) (*Machine, *Owner, error) {
	cfg.SchemaVersion = SchemaVersion
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = time.Now().UTC()
	}
	if err := cfg.validate(); err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create library: %w", err)
	}
	dir := filepath.Join(l.Dir, cfg.Name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, nil, fmt.Errorf("%w: %s", ErrExists, dir)
		}
		return nil, nil, fmt.Errorf("create machine: %w", err)
	}
	m := &Machine{Name: cfg.Name, Dir: dir, Config: cfg}
	owner, err := m.Lock(purpose)
	if err != nil {
		_ = os.RemoveAll(dir) //nolint:gosec // the directory this call created
		return nil, nil, err
	}
	// abandon removes the directory this call created exclusively, so a failed
	// create leaves no half-made machine behind.
	abandon := func(err error) (*Machine, *Owner, error) {
		_ = os.RemoveAll(dir) //nolint:gosec // the directory this call created
		owner.Release()
		return nil, nil, err
	}
	for _, sub := range []string{stagesDir, attemptsDir} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
			return abandon(fmt.Errorf("create machine: %w", err))
		}
	}
	if err := writeJSONAtomic(filepath.Join(dir, configFile), cfg); err != nil {
		return abandon(err)
	}
	return m, owner, nil
}

// Open loads the named machine and validates its configuration.
func (l Library) Open(name string) (*Machine, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	dir := filepath.Join(l.Dir, name)
	path := filepath.Join(dir, configFile)
	data, err := os.ReadFile(path) //nolint:gosec // inside the library
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(dir); statErr == nil { //nolint:gosec // name checked by validName
			return nil, fmt.Errorf("%w: %s has no %s; restore it from a backup or remove the directory (a replacement identity is never generated)", ErrCorrupt, dir, configFile)
		}
		return nil, fmt.Errorf("%w: %s in %s", ErrNotFound, name, l.Dir)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, corrupt(path, err)
	}
	if probe.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: %s has schema version %d; this build reads version %d", ErrUnsupportedSchema, path, probe.SchemaVersion, SchemaVersion)
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, corrupt(path, err)
	}
	if cfg.Name != name {
		return nil, corrupt(path, fmt.Errorf("%w: it names machine %q", ErrInvalidConfig, cfg.Name))
	}
	if err := cfg.validate(); err != nil {
		return nil, corrupt(path, err)
	}
	return &Machine{Name: name, Dir: dir, Config: cfg}, nil
}

func corrupt(path string, err error) error {
	return fmt.Errorf("%w: %s: %w; restore it from a backup or remove the machine (a replacement identity is never generated)", ErrCorrupt, path, err)
}

// Entry is one machine found in a library, or why it could not be opened.
type Entry struct {
	Name    string
	Machine *Machine
	Err     error
}

// List returns every machine directory in the library, including ones that
// cannot be opened, in name order. A missing library is empty.
func (l Library) List() ([]Entry, error) {
	dirents, err := os.ReadDir(l.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read library: %w", err)
	}
	var entries []Entry
	for _, d := range dirents {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		m, err := l.Open(d.Name())
		entries = append(entries, Entry{Name: d.Name(), Machine: m, Err: err})
	}
	return entries, nil
}

// Attempt is one run of a stage. Attempts are recorded separately from
// completed-stage receipts.
type Attempt struct {
	m     *Machine
	owner *Owner
	rec   attemptRecord
}

type attemptRecord struct {
	SchemaVersion int        `json:"schema_version"`
	ID            string     `json:"id"`
	Stage         Stage      `json:"stage"`
	Status        Status     `json:"status"`
	PID           int        `json:"pid"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type receiptRecord struct {
	SchemaVersion int        `json:"schema_version"`
	Stage         Stage      `json:"stage"`
	Attempt       string     `json:"attempt"`
	CompletedAt   time.Time  `json:"completed_at"`
	Evidence      []Evidence `json:"evidence"`
}

// Begin starts an attempt of stage. Starting a stage withdraws the receipts of
// that stage and every later one, whose results depended on it.
func (m *Machine) Begin(o *Owner, stage Stage) (*Attempt, error) {
	if err := o.owns(m); err != nil {
		return nil, err
	}
	i := slices.Index(Stages, stage)
	if i < 0 {
		return nil, fmt.Errorf("%w: unknown stage %q", ErrInvalidRecord, stage)
	}
	for _, later := range Stages[i:] {
		if err := os.Remove(m.receiptPath(later)); err != nil && !errors.Is(err, fs.ErrNotExist) { //nolint:gosec // receipt in the owned machine directory
			return nil, fmt.Errorf("withdraw %s receipt: %w", later, err)
		}
	}
	now := time.Now().UTC()
	a := &Attempt{m: m, owner: o, rec: attemptRecord{
		SchemaVersion: SchemaVersion,
		ID:            strconv.FormatInt(now.UnixNano(), 10) + "-" + string(stage),
		Stage:         stage, Status: StatusRunning, PID: os.Getpid(), StartedAt: now,
	}}
	if err := a.write(); err != nil {
		return nil, err
	}
	return a, nil
}

// Complete records the stage as completed with the given artifacts, named by
// path relative to the machine directory. Their digests are computed here, so
// a receipt only exists once its evidence does.
func (a *Attempt) Complete(artifacts ...string) error {
	if err := a.owner.owns(a.m); err != nil {
		return err
	}
	if a.rec.Status != StatusRunning {
		return fmt.Errorf("%w: %s attempt is already %s", ErrInvalidRecord, a.rec.Stage, a.rec.Status)
	}
	evidence := make([]Evidence, 0, len(artifacts))
	for _, name := range artifacts {
		e, err := a.m.digest(name)
		if err != nil {
			return err
		}
		evidence = append(evidence, e)
	}
	rec := StageRecord{Stage: a.rec.Stage, Status: StatusCompleted, Evidence: evidence}
	if err := rec.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	receipt := receiptRecord{SchemaVersion: SchemaVersion, Stage: a.rec.Stage, Attempt: a.rec.ID, CompletedAt: now, Evidence: evidence}
	if err := writeJSONAtomic(a.m.receiptPath(a.rec.Stage), receipt); err != nil {
		return err
	}
	return a.end(StatusCompleted, nil)
}

// Fail records the attempt as failed with its cause. No receipt is written.
func (a *Attempt) Fail(cause error) error { return a.end(StatusFailed, cause) }

// Cancel records the attempt as canceled. No receipt is written.
func (a *Attempt) Cancel(cause error) error { return a.end(StatusCanceled, cause) }

func (a *Attempt) end(status Status, cause error) error {
	if err := a.owner.owns(a.m); err != nil {
		return err
	}
	if a.rec.Status != StatusRunning {
		return fmt.Errorf("%w: %s attempt is already %s", ErrInvalidRecord, a.rec.Stage, a.rec.Status)
	}
	if cause == nil && status != StatusCompleted {
		cause = errors.New(string(status)) //nolint:err113 // records an unexplained stop
	}
	now := time.Now().UTC()
	a.rec.Status, a.rec.EndedAt = status, &now
	if cause != nil {
		a.rec.Error = cause.Error()
	}
	return a.write()
}

func (a *Attempt) write() error {
	return writeJSONAtomic(filepath.Join(a.m.Dir, attemptsDir, a.rec.ID+".json"), a.rec)
}

func (m *Machine) receiptPath(stage Stage) string {
	return filepath.Join(m.Dir, stagesDir, string(stage)+".json")
}

func (m *Machine) digest(name string) (Evidence, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if name == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return Evidence{}, fmt.Errorf("%w: evidence %q is outside the machine directory", ErrInvalidRecord, name)
	}
	f, err := os.Open(filepath.Join(m.Dir, clean)) //nolint:gosec // checked to stay inside the machine directory
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Evidence{}, fmt.Errorf("%w: %s: %s", ErrArtifactMissing, m.Name, name)
		}
		return Evidence{}, fmt.Errorf("open evidence: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return Evidence{}, fmt.Errorf("stat evidence: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Evidence{}, fmt.Errorf("%w: %s: %s is not a regular file", ErrInvalidRecord, m.Name, name)
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Evidence{}, fmt.Errorf("read evidence: %w", err)
	}
	return Evidence{Name: filepath.ToSlash(clean), SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

// StageState is what is known about one stage: a verified receipt, a problem
// that keeps a receipt from counting, and the latest attempt.
type StageState struct {
	Stage Stage
	// Completed is true only for a receipt whose evidence still matches.
	Completed bool
	Receipt   *StageRecord
	// Problem explains why a receipt does not count.
	Problem error
	// Latest is the most recent attempt, if any. A running attempt whose
	// process no longer owns the machine was interrupted.
	Latest *StageRecord
}

// Stages reports every pipeline stage in order.
func (m *Machine) Stages() ([]StageState, error) {
	attempts, err := m.attempts()
	if err != nil {
		return nil, err
	}
	states := make([]StageState, 0, len(Stages))
	for _, stage := range Stages {
		s := StageState{Stage: stage}
		if a, ok := attempts[stage]; ok {
			s.Latest = &StageRecord{Stage: stage, Status: a.Status, Error: a.Error}
		}
		rec, found, problem := m.receipt(stage)
		if found {
			s.Receipt, s.Problem = &rec, problem
			s.Completed = problem == nil
		} else if problem != nil {
			s.Problem = problem
		}
		states = append(states, s)
	}
	return states, nil
}

// receipt reads stage's receipt. found reports whether a receipt file exists;
// problem explains why an existing receipt does not count.
func (m *Machine) receipt(stage Stage) (rec StageRecord, found bool, problem error) {
	path := m.receiptPath(stage)
	data, err := os.ReadFile(path) //nolint:gosec // inside the machine directory
	if errors.Is(err, fs.ErrNotExist) {
		return StageRecord{}, false, nil
	}
	if err != nil {
		return StageRecord{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	var r receiptRecord
	if err := json.Unmarshal(data, &r); err != nil || r.SchemaVersion != SchemaVersion || r.Stage != stage {
		return StageRecord{}, false, fmt.Errorf("%w: %s is not a valid %s receipt; run the stage again", ErrCorrupt, path, stage)
	}
	rec = StageRecord{Stage: stage, Status: StatusCompleted, Evidence: r.Evidence}
	if err := rec.Validate(); err != nil {
		return rec, true, fmt.Errorf("%w: %s: %w", ErrCorrupt, path, err)
	}
	for _, e := range r.Evidence {
		got, err := m.digest(e.Name)
		if err != nil {
			return rec, true, err
		}
		if got != e {
			return rec, true, fmt.Errorf("%w: %s: %s is %d bytes with SHA-256 %s, recorded %d bytes with %s; run the %s stage again",
				ErrArtifactChanged, m.Name, e.Name, got.Size, got.SHA256, e.Size, e.SHA256, stage)
		}
	}
	return rec, true, nil
}

// attempts returns the latest attempt of each stage.
func (m *Machine) attempts() (map[Stage]attemptRecord, error) {
	dir := filepath.Join(m.Dir, attemptsDir)
	latest := map[Stage]attemptRecord{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return latest, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read attempts: %w", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tempPrefix) || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // inside the machine directory
		if err != nil {
			return nil, fmt.Errorf("read attempt: %w", err)
		}
		var a attemptRecord
		if err := json.Unmarshal(data, &a); err != nil || !slices.Contains(Stages, a.Stage) {
			return nil, fmt.Errorf("%w: %s", ErrCorrupt, filepath.Join(dir, e.Name()))
		}
		if prev, ok := latest[a.Stage]; !ok || a.StartedAt.After(prev.StartedAt) {
			latest[a.Stage] = a
		}
	}
	return latest, nil
}

type nativeRecord struct {
	State      host.State `json:"state"`
	ObservedAt time.Time  `json:"observed_at"`
}

// RecordNative stores the latest native state observed by the owner.
func (m *Machine) RecordNative(o *Owner, state host.State) error {
	if err := o.owns(m); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(m.Dir, nativeFile), nativeRecord{State: state, ObservedAt: time.Now().UTC()})
}

// LastNative returns the last recorded native observation. It describes the
// past, not necessarily the current state.
func (m *Machine) LastNative() (host.State, time.Time, error) {
	data, err := os.ReadFile(filepath.Join(m.Dir, nativeFile))
	if errors.Is(err, fs.ErrNotExist) {
		return host.StateUnknown, time.Time{}, nil
	}
	if err != nil {
		return host.StateUnknown, time.Time{}, fmt.Errorf("read native observation: %w", err)
	}
	var r nativeRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return host.StateUnknown, time.Time{}, fmt.Errorf("%w: %s", ErrCorrupt, nativeFile)
	}
	return r.State, r.ObservedAt, nil
}

// Observe returns the machine's recorded observations. Guest readiness is never
// stored, so it is always unknown here.
func (m *Machine) Observe() (Observation, error) {
	native, _, err := m.LastNative()
	if err != nil {
		return Observation{}, err
	}
	states, err := m.Stages()
	if err != nil {
		return Observation{}, err
	}
	o := Observation{Native: native, Guest: guest.ReadinessUnknown}
	for _, s := range states {
		if s.Completed {
			o.Stages = append(o.Stages, *s.Receipt)
		} else if s.Latest != nil && s.Latest.Status != StatusCompleted {
			o.Stages = append(o.Stages, *s.Latest)
		}
	}
	return o, nil
}
