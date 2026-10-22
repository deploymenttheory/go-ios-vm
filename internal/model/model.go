// Package model contains firmware evidence types without transport or storage dependencies.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const SchemaVersion = 1

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
type Run struct {
	ID       string     `json:"id"`
	TargetID string     `json:"target_id"`
	State    string     `json:"state"`
	Started  time.Time  `json:"started"`
	Coverage []Coverage `json:"coverage"`
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

func NewObservation(t Target, subject, property string, value any, source Location, evidence Blob, extractor string) (Observation, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return Observation{}, err
	}
	o := Observation{TargetID: t.ID, Subject: subject, Property: property, Presence: "present", Value: b, Source: source, Extractor: extractor, EvidenceID: evidence.id, Evidence: evidence.data}
	location, err := json.Marshal(source)
	if err != nil {
		return Observation{}, err
	}
	o.ID = ID(t.ID, subject, property, string(b), string(location), o.EvidenceID, extractor)
	return o, o.ValidateMetadata()
}
func (o Observation) ValidateMetadata() error {
	if o.ID == "" || o.TargetID == "" || o.Extractor == "" || o.Subject == "" || o.Property == "" || len(o.EvidenceID) != 64 || o.Source.ArtifactID == "" || o.Source.Member == "" || o.Source.Entry < 0 {
		return fmt.Errorf("observation identity/source is incomplete")
	}
	switch o.Source.Coordinate {
	case "plist-selector", "decoded-payload-byte", "zip-member-byte-ranges":
	default:
		return fmt.Errorf("unsupported source coordinate %q", o.Source.Coordinate)
	}
	if o.Source.Offset+o.Source.Length < o.Source.Offset {
		return fmt.Errorf("source range overflow")
	}
	if o.Presence != "present" && o.Presence != "absent" && o.Presence != "unexamined" {
		return fmt.Errorf("invalid presence %q", o.Presence)
	}
	if !json.Valid(o.Value) {
		return fmt.Errorf("invalid typed observation value")
	}
	return nil
}
func (o Observation) Validate() error {
	if err := o.ValidateMetadata(); err != nil {
		return err
	}
	if Digest(o.Evidence) != o.EvidenceID {
		return fmt.Errorf("evidence digest mismatch")
	}
	return nil
}
