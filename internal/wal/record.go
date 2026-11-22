package wal

// Operation constants for WAL records.
// Using typed constants keeps us from passing raw strings around the codebase.
const (
	OperationSet    = "SET"
	OperationDelete = "DELETE"
)

// Record represents a single entry in the Write-Ahead Log.
//
// Fields:
//   - Sequence: monotonically increasing counter; lets us detect gaps/ordering issues.
//   - Operation: "SET" or "DELETE".
//   - Key: the database key this operation targets.
//   - Value: only present for SET operations; omitted (empty string) for DELETE.
//
// Serialization: JSON Lines (one JSON object per line in the WAL file).
// The `omitempty` tag on Value means DELETE records won't carry a "value" field,
// keeping the log compact and self-documenting.
type Record struct {
	Sequence  uint64 `json:"sequence"`
	Operation string `json:"operation"`
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
}
