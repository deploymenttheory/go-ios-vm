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
