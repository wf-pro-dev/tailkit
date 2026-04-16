package types

import "encoding/json"

const (
	EventError = "error"

	EventJobStdout    = "job.stdout"
	EventJobStderr    = "job.stderr"
	EventJobStatus    = "job.status"
	EventJobCompleted = "job.completed"
	EventJobFailed    = "job.failed"

	EventLogLine       = "log.line"
	EventStatsSnapshot = "stats.snapshot"

	EventJournalEntry = "journal.entry"

	EventCPU       = "metrics.cpu"
	EventMemory    = "metrics.memory"
	EventNetwork   = "metrics.network"
	EventProcesses = "metrics.processes"
	EventAll       = "metrics.all"

	EventPortsSnapshot = "ports.snapshot"
	EventPortBound     = "port.bound"
	EventPortReleased  = "port.released"
)

// Event is one typed server-sent event received from a tailkitd stream.
type Event[T any] struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
	Data T      `json:"data"`
}

// RawEvent is the parser-side SSE envelope used before typed decoding.
type RawEvent struct {
	Name string
	ID   int64
	Data json.RawMessage
}
