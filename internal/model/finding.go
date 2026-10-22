package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var errInvalidRevision = errors.New("invalid finding revision")

// BasisReference names one retained observation or one earlier revision.
type BasisReference struct {
	Role          string       `json:"role"`
	ObservationID string       `json:"observation_id,omitempty"`
	RevisionID    string       `json:"revision_id,omitempty"`
	Observation   *Observation `json:"observation,omitempty"`
}

type RevisionDraft struct {
	TargetID      string           `json:"target_id"`
	Subject       string           `json:"subject"`
	Property      string           `json:"property"`
	Basis         string           `json:"basis"`
	Presence      string           `json:"presence,omitempty"`
	Value         json.RawMessage  `json:"value,omitempty"`
	Rationale     string           `json:"rationale"`
	Author        string           `json:"author"`
	Supersedes    string           `json:"supersedes,omitempty"`
	ObservationID string           `json:"observation_id,omitempty"`
	References    []BasisReference `json:"references,omitempty"`
}

func (d RevisionDraft) Validate() error {
	if d.TargetID == "" || strings.TrimSpace(d.Subject) == "" ||
		strings.TrimSpace(d.Property) == "" || strings.TrimSpace(d.Rationale) == "" ||
		strings.TrimSpace(d.Author) == "" {
		return fmt.Errorf(
			"%w: target, subject, property, rationale and author are required",
			errInvalidRevision,
		)
	}
	if len(d.Value) > 0 && !json.Valid(d.Value) {
		return fmt.Errorf("%w: value must be JSON", errInvalidRevision)
	}
	supports := 0
	for _, ref := range d.References {
		if ref.Role != "supporting" && ref.Role != "contradicting" {
			return fmt.Errorf("%w: invalid reference role", errInvalidRevision)
		}
		if (ref.ObservationID == "") == (ref.RevisionID == "") {
			return fmt.Errorf(
				"%w: reference must name exactly one observation or revision",
				errInvalidRevision,
			)
		}
		if ref.Role == "supporting" {
			supports++
		}
	}
	switch d.Basis {
	case "observed":
		if d.ObservationID == "" || d.Presence != "" || len(d.Value) != 0 {
			return fmt.Errorf(
				"%w: observed value and presence come from --observation",
				errInvalidRevision,
			)
		}
	case "inferred":
		if d.ObservationID != "" || supports == 0 {
			return fmt.Errorf(
				"%w: inference needs supporting references, not a direct observation",
				errInvalidRevision,
			)
		}
		if d.Presence != "present" && d.Presence != "absent" && d.Presence != "unexamined" {
			return fmt.Errorf("%w: invalid inferred presence", errInvalidRevision)
		}
		if (d.Presence == "present") != (len(d.Value) > 0) {
			return fmt.Errorf(
				"%w: present inference needs a value; other states have none",
				errInvalidRevision,
			)
		}
		if d.Presence == "present" && bytes.Equal(bytes.TrimSpace(d.Value), []byte("null")) {
			return fmt.Errorf(
				"%w: present inference needs a concrete typed value",
				errInvalidRevision,
			)
		}
	case "unknown":
		if d.ObservationID != "" || d.Presence != "" || len(d.Value) != 0 {
			return fmt.Errorf("%w: unknown has no direct observation or answer", errInvalidRevision)
		}
	default:
		return fmt.Errorf("%w: basis must be observed, inferred or unknown", errInvalidRevision)
	}
	return nil
}

// RevisionReceipt identifies a committed revision without a second database read.
type RevisionReceipt struct {
	FindingID  string `json:"finding_id"`
	RevisionID string `json:"revision_id"`
}

type FindingRevision struct {
	ID          string           `json:"id"`
	Basis       string           `json:"basis"`
	Presence    string           `json:"presence"`
	Value       json.RawMessage  `json:"value,omitempty"`
	Rationale   string           `json:"rationale"`
	Author      string           `json:"author"`
	Created     time.Time        `json:"created"`
	Supersedes  string           `json:"supersedes,omitempty"`
	Assessment  string           `json:"assessment"`
	Observation *Observation     `json:"observation,omitempty"`
	References  []BasisReference `json:"references"`
}

type FindingChain struct {
	ID        string            `json:"id"`
	TargetID  string            `json:"target_id"`
	Subject   string            `json:"subject"`
	Property  string            `json:"property"`
	Revisions []FindingRevision `json:"revisions"`
}
