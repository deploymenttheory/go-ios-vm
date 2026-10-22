package model

import "testing"

func TestBorrowedEvidenceMutationAndRangeOverflow(t *testing.T) {
	data := []byte("source")
	source := Location{
		ArtifactID: "artifact",
		Member:     "tree",
		Coordinate: "decoded-payload-byte",
		Length:     6,
	}
	o, err := NewObservation(
		Target{ID: "target"},
		"node",
		"property",
		0,
		source,
		NewBlob(data),
		"fixture",
	)
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	if err = o.Validate(); err == nil {
		t.Fatal("mutation escaped integrity validation")
	}
	source.Offset = ^uint64(0)
	source.Length = 2
	if _, err = NewObservation(
		Target{ID: "target"},
		"node",
		"property",
		0,
		source,
		NewBlob(data),
		"fixture",
	); err == nil {
		t.Fatal("overflowing byte range accepted")
	}
}

func TestObservationPresenceHasDistinctIdentity(t *testing.T) {
	target := Target{ID: "target"}
	loc := Location{ArtifactID: "artifact", Member: "OS.dmg", Coordinate: "apfs-file-object"}
	blob := NewBlob([]byte("searched path"))
	present, err := NewObservation(target, "file", "search", "path", loc, blob, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	absent, err := NewObservationWithPresence(target, "file", "search", "absent", "path", loc, blob, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if present.ID == absent.ID || absent.Presence != "absent" {
		t.Fatal("present and absent readings collapsed")
	}
	if _, err := NewObservationWithPresence(target, "file", "search", "unsupported", "path", loc, blob, "fixture"); err == nil {
		t.Fatal("unsupported presence accepted")
	}
}
