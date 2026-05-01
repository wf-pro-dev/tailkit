package types

import (
	"time"

	"github.com/docker/docker/api/types/events"
	gopsutilcpu "github.com/shirou/gopsutil/v4/cpu"
	gopsutildisk "github.com/shirou/gopsutil/v4/disk"
	gopsutilhost "github.com/shirou/gopsutil/v4/host"
	gopsutilmem "github.com/shirou/gopsutil/v4/mem"
	gopsutilnet "github.com/shirou/gopsutil/v4/net"
)

type DockerEvent = events.Message

// JobUpdate is the typed payload emitted by exec job streams.
// Event identifies the originating SSE event name.
type JobUpdate struct {
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
}

// CPU bundles per-CPU usage percentages with static CPU info.
type CPU struct {
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

type Process struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryRSS  uint64  `json:"memory_rss_bytes"`
	Cmdline    string  `json:"cmdline"`
}

type Memory struct {
	Virtual *gopsutilmem.VirtualMemoryStat `json:"virtual"`
	Swap    *gopsutilmem.SwapMemoryStat    `json:"swap"`
}

type Port struct {
	Addr    string `json:"addr"`
	Port    uint16 `json:"port"`
	Proto   string `json:"proto"`
	PID     int    `json:"pid"`
	Process string `json:"process"`
}

// Metrics is the typed payload emitted by the metrics all stream.
type Metrics struct {
	Host      *gopsutilhost.InfoStat       `json:"host,omitempty"`
	CPU       *CPU                         `json:"cpu,omitempty"`
	Memory    *Memory                      `json:"memory,omitempty"`
	Disk      []*gopsutildisk.UsageStat    `json:"disk,omitempty"`
	Network   []gopsutilnet.IOCountersStat `json:"network,omitempty"`
	Processes []Process                    `json:"processes,omitempty"`
	Ports     []Port                       `json:"ports,omitempty"`
} // metrics/handler328

// Port describes one TCP socket in the LISTEN state.

// PortUpdate is the typed payload emitted by the metrics ports stream.
// Snapshot events populate Ports; delta events populate Port.
type PortUpdate struct {
	Kind  string `json:"kind"`
	Port  Port   `json:"port,omitempty"`
	Ports []Port `json:"ports,omitempty"`
}
