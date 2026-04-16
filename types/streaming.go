package types

import (
	"time"

	gopsutilcpu "github.com/shirou/gopsutil/v4/cpu"
	gopsutildisk "github.com/shirou/gopsutil/v4/disk"
	gopsutilhost "github.com/shirou/gopsutil/v4/host"
	gopsutilmem "github.com/shirou/gopsutil/v4/mem"
	gopsutilnet "github.com/shirou/gopsutil/v4/net"
)

// JobEvent is the typed payload emitted by exec job streams.
// Event identifies the originating SSE event name.
type JobEvent struct {
	Event    string    `json:"-"`
	JobID    string    `json:"job_id"`
	Line     string    `json:"line,omitempty"`
	Stream   string    `json:"stream,omitempty"`
	Status   JobStatus `json:"status,omitempty"`
	ExitCode int       `json:"exit_code,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// LogLine is emitted by Docker log streams.
type LogLine struct {
	ContainerID string    `json:"container_id"`
	Stream      string    `json:"stream"`
	TS          time.Time `json:"ts"`
	Line        string    `json:"line"`
} // streams.go:18

// CPUResult bundles per-CPU usage percentages with static CPU info.
type CPUResult struct {
	Info    []gopsutilcpu.InfoStat `json:"info"`
	Percent []float64              `json:"percent_per_cpu"`
	Total   float64                `json:"percent_total"`
}

// JournalEntry is one systemd journal entry.
type JournalEntry struct {
	Timestamp uint64            `json:"timestamp_us"` // realtime timestamp in microseconds
	Message   string            `json:"message"`
	Unit      string            `json:"unit,omitempty"`
	Priority  string            `json:"priority,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type ProcessStat struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryRSS  uint64  `json:"memory_rss_bytes"`
	Cmdline    string  `json:"cmdline"`
}

type MemoryResult struct {
	Virtual *gopsutilmem.VirtualMemoryStat `json:"virtual"`
	Swap    *gopsutilmem.SwapMemoryStat    `json:"swap"`
}

type ListenPort struct {
	Addr    string `json:"addr"`
	Port    uint16 `json:"port"`
	Proto   string `json:"proto"`
	PID     int    `json:"pid"`
	Process string `json:"process"`
}

// AllMetrics is the typed payload emitted by the metrics all stream.
type AllMetrics struct {
	Host      *gopsutilhost.InfoStat       `json:"host,omitempty"`
	CPU       *CPUResult                   `json:"cpu,omitempty"`
	Memory    *MemoryResult                `json:"memory,omitempty"`
	Disk      []*gopsutildisk.UsageStat    `json:"disk,omitempty"`
	Network   []gopsutilnet.IOCountersStat `json:"network,omitempty"`
	Processes []ProcessStat                `json:"processes,omitempty"`
	Ports     []ListenPort                 `json:"ports,omitempty"`
} // metrics/handler328

// ListenPort describes one TCP socket in the LISTEN state.

// PortEvent is the typed payload emitted by the metrics ports stream.
// Snapshot events populate Ports; delta events populate Port.
type PortEvent struct {
	Kind  string       `json:"kind"`
	Port  ListenPort   `json:"port,omitempty"`
	Ports []ListenPort `json:"ports,omitempty"`
}
