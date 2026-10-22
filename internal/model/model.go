// Package model contains firmware evidence types without transport or storage dependencies.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	errIncompleteObservation = errors.New(
		"observation identity/source is incomplete",
	)
	errUnsupportedSourceCoordinate  = errors.New("unsupported source coordinate")
	errSourceRangeOverflow          = errors.New("source range overflow")
	errInvalidPresence              = errors.New("invalid presence")
	errInvalidTypedObservationValue = errors.New("invalid typed observation value")
	errEvidenceDigestMismatch       = errors.New("evidence digest mismatch")
)

const (
	SchemaVersion           = 1
	MaxObservationsPerBatch = 128
)

type Artifact struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Path   string `json:"path,omitempty"`
}
type Target struct {
	ID         string            `json:"id"`
	ArtifactID string            `json:"artifact_id"`
	Identity   int               `json:"identity"`
	Products   []string          `json:"products"`
	Board      string            `json:"board"`
	Version    string            `json:"version"`
	Build      string            `json:"build"`
	Variant    string            `json:"variant"`
	Components map[string]string `json:"components"`
}
type Location struct {
	ArtifactID string   `json:"artifact_id"`
	Member     string   `json:"member"`
	Entry      int      `json:"entry"`
	Coordinate string   `json:"coordinate"`
	Selector   string   `json:"selector"`
	Offset     uint64   `json:"offset,string"`
	Length     uint64   `json:"length,string"`
	Transforms []string `json:"transforms,omitempty"`
}
type Observation struct {
	ID         string          `json:"id"`
	TargetID   string          `json:"target_id"`
	Subject    string          `json:"subject"`
	Property   string          `json:"property"`
	Presence   string          `json:"presence"`
	Value      json.RawMessage `json:"value"`
	Source     Location        `json:"source"`
	Extractor  string          `json:"extractor"`
	EvidenceID string          `json:"evidence_id"`
	Evidence   []byte          `json:"-"`
}
type Coverage struct {
	Component string `json:"component"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
}
type ObservationBatch struct {
	Component    string        `json:"component"`
	Ordinal      int           `json:"ordinal"`
	Observations []Observation `json:"observations"`
	Final        *Coverage     `json:"final,omitempty"`
}
type Run struct {
	ID          string     `json:"id"`
	TargetID    string     `json:"target_id"`
	State       string     `json:"state"`
	Started     time.Time  `json:"started"`
	InputSHA256 string     `json:"input_sha256,omitempty"`
	InputSize   int64      `json:"input_size,omitempty"`
	Extractor   string     `json:"extractor,omitempty"`
	Planned     []string   `json:"planned_components,omitempty"`
	Coverage    []Coverage `json:"coverage"`
}
type Finding struct {
	ID          string      `json:"id"`
	RunID       string      `json:"run_id"`
	Observation Observation `json:"observation"`
	Basis       string      `json:"basis"`
	Assessment  string      `json:"assessment"`
}
type Change struct {
	Subject  string       `json:"subject"`
	Property string       `json:"property"`
	State    string       `json:"state"`
	Left     *Observation `json:"left,omitempty"`
	Right    *Observation `json:"right,omitempty"`
}

func Digest(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func ID(parts ...string) string { b, _ := json.Marshal(parts); return Digest(b) }

// Blob borrows immutable supporting bytes and hashes them once for many observations.
// Publication revalidates each distinct blob before committing it.
type Blob struct {
	id   string
	data []byte
}

func NewBlob(data []byte) Blob { return Blob{id: Digest(data), data: data} }

func NewObservation(
	t Target,
	subject, property string,
	value any,
	source Location,
	evidence Blob,
	extractor string,
) (Observation, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return Observation{}, fmt.Errorf("NewObservation: json.Marshal: %w", err)
	}
	o := Observation{
		TargetID:   t.ID,
		Subject:    subject,
		Property:   property,
		Presence:   "present",
		Value:      b,
		Source:     source,
		Extractor:  extractor,
		EvidenceID: evidence.id,
		Evidence:   evidence.data,
	}
	location, err := json.Marshal(source)
	if err != nil {
		return Observation{}, fmt.Errorf("NewObservation: json.Marshal: %w", err)
	}
	o.ID = ID(t.ID, subject, property, string(b), string(location), o.EvidenceID, extractor)
	return o, o.ValidateMetadata()
}

func (o Observation) ValidateMetadata() error {
	if o.ID == "" || o.TargetID == "" || o.Extractor == "" || o.Subject == "" || o.Property == "" ||
		len(o.EvidenceID) != 64 ||
		o.Source.ArtifactID == "" ||
		o.Source.Member == "" ||
		o.Source.Entry < 0 {
		return errIncompleteObservation
	}
	switch o.Source.Coordinate {
	case "plist-selector", "decoded-payload-byte", "zip-member-byte-ranges", "zip-encoded-member":
	default:
		return fmt.Errorf("%w: %q", errUnsupportedSourceCoordinate, o.Source.Coordinate)
	}
	if o.Source.Offset+o.Source.Length < o.Source.Offset {
		return errSourceRangeOverflow
	}
	if o.Presence != "present" && o.Presence != "absent" && o.Presence != "unexamined" {
		return fmt.Errorf("%w: %q", errInvalidPresence, o.Presence)
	}
	if !json.Valid(o.Value) {
		return errInvalidTypedObservationValue
	}
	return nil
}

func (o Observation) Validate() error {
	if err := o.ValidateMetadata(); err != nil {
		return err
	}
	if Digest(o.Evidence) != o.EvidenceID {
		return errEvidenceDigestMismatch
	}
	return nil
}
