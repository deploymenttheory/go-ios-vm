// Package progress defines the typed progress events that long-running
// operations report. Progress describes work in flight; it is never evidence
// that a stage completed.
package progress

// Event is one observation of work in flight.
type Event struct {
	// Operation names the reporting operation, for example "restore".
	Operation string
	// Step names the current step within the operation.
	Step string
	// Done and Total count work units. Total is zero when unknown.
	Done, Total int64
	// Unit names what Done and Total count, for example "bytes".
	Unit string
	// Message is optional human-readable detail.
	Message string
}

// Reporter receives progress events. Implementations must be safe for
// concurrent use and must not block the reporting operation for long.
type Reporter interface {
	Report(Event)
}

// ReporterFunc adapts a function to Reporter.
type ReporterFunc func(Event)

// Report calls f(e).
func (f ReporterFunc) Report(e Event) { f(e) }

// Discard ignores every event.
var Discard Reporter = ReporterFunc(func(Event) {})
