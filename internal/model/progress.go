package model

import "time"

// Progress is the latest committed snapshot for one acquisition attempt or
// inspection run. Totals are nil until the producer knows them.
type Progress struct {
	ID                string     `json:"id"`
	Kind              string     `json:"kind"`
	RunID             string     `json:"run_id,omitempty"`
	InputSHA256       string     `json:"input_sha256,omitempty"`
	InputSize         *int64     `json:"input_size,omitempty"`
	SourceFingerprint string     `json:"source_fingerprint,omitempty"`
	Phase             string     `json:"phase"`
	State             string     `json:"state"`
	Component         string     `json:"component,omitempty"`
	CompletedBytes    int64      `json:"completed_bytes"`
	VerifiedBytes     int64      `json:"verified_bytes"`
	TotalBytes        *int64     `json:"total_bytes,omitempty"`
	CompletedUnits    int64      `json:"completed_units"`
	TotalUnits        *int64     `json:"total_units,omitempty"`
	PublishedBatches  int64      `json:"published_batches"`
	DiagnosticCode    string     `json:"diagnostic_code,omitempty"`
	Sequence          int64      `json:"sequence"`
	Version           int64      `json:"version"`
	Started           time.Time  `json:"started"`
	Updated           time.Time  `json:"updated"`
	Finished          *time.Time `json:"finished,omitempty"`
}

type ProgressEvent struct {
	OperationID    string    `json:"operation_id"`
	Sequence       int64     `json:"sequence"`
	Phase          string    `json:"phase"`
	State          string    `json:"state"`
	Component      string    `json:"component,omitempty"`
	DiagnosticCode string    `json:"diagnostic_code,omitempty"`
	Created        time.Time `json:"created"`
}
