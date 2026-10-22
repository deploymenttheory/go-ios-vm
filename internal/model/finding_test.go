package model

import (
	"encoding/json"
	"testing"
)

func TestRevisionDraftBasisRules(t *testing.T) {
	base := RevisionDraft{
		TargetID:  "target",
		Subject:   "device",
		Property:  "address",
		Rationale: "test reason",
		Author:    "test",
	}
	for _, tc := range []struct {
		name  string
		draft RevisionDraft
		valid bool
	}{
		{"observed", RevisionDraft{Basis: "observed", ObservationID: "obs"}, true},
		{"invented observed value", RevisionDraft{Basis: "observed", ObservationID: "obs", Value: json.RawMessage(`42`)}, false},
		{"inference without support", RevisionDraft{Basis: "inferred", Presence: "present", Value: json.RawMessage(`42`)}, false},
		{"supported inference", RevisionDraft{Basis: "inferred", Presence: "present", Value: json.RawMessage(`42`), References: []BasisReference{{Role: "supporting", ObservationID: "obs"}}}, true},
		{"null is not a present value", RevisionDraft{Basis: "inferred", Presence: "present", Value: json.RawMessage(` null `), References: []BasisReference{{Role: "supporting", ObservationID: "obs"}}}, false},
		{"unknown", RevisionDraft{Basis: "unknown"}, true},
		{"unknown claiming direct evidence", RevisionDraft{Basis: "unknown", ObservationID: "obs"}, false},
		{"ambiguous reference", RevisionDraft{Basis: "inferred", Presence: "absent", References: []BasisReference{{Role: "supporting", ObservationID: "obs", RevisionID: "rev"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			d.Basis = tc.draft.Basis
			d.ObservationID = tc.draft.ObservationID
			d.Presence = tc.draft.Presence
			d.Value = tc.draft.Value
			d.References = tc.draft.References
			if (d.Validate() == nil) != tc.valid {
				t.Fatalf("unexpected validity: %+v", d)
			}
		})
	}
}
