package types

// MetricsConfig is the parsed and validated representation of metrics.toml.
//
// Each sub-section maps to one metrics endpoint group. Sections are
// independent — enabling disk does not require enabling cpu, and so on.
type MetricsConfig struct {
	Enabled   bool
	Host      HostMetricsConfig    `toml:"host"`
	CPU       CPUMetricsConfig     `toml:"cpu"`
	Memory    MemoryMetricsConfig  `toml:"memory"`
	Disk      DiskMetricsConfig    `toml:"disk"`
	Network   NetworkMetricsConfig `toml:"network"`
	Processes ProcessMetricsConfig `toml:"processes"`
	Ports     PortMetricsConfig    `toml:"ports"`
}

// HostMetricsConfig controls GET /integrations/metrics/host.
type HostMetricsConfig struct {
	Enabled bool `toml:"enabled"`
}

// CPUMetricsConfig controls GET /integrations/metrics/cpu.
type CPUMetricsConfig struct {
	Enabled bool `toml:"enabled"`
}

// MemoryMetricsConfig controls GET /integrations/metrics/memory.
type MemoryMetricsConfig struct {
	Enabled bool `toml:"enabled"`
}

// DiskMetricsConfig controls GET /integrations/metrics/disk.
type DiskMetricsConfig struct {
	Enabled bool `toml:"enabled"`

	// Paths restricts disk stats to specific mount points.
	// All entries must be absolute paths.
	// If empty, all mounted filesystems are reported.
	Paths []string `toml:"paths"`
}

// NetworkMetricsConfig controls GET /integrations/metrics/network.
type NetworkMetricsConfig struct {
	Enabled bool `toml:"enabled"`

	// Interfaces restricts stats to specific network interfaces by name.
	// If empty, all interfaces are reported.
	Interfaces []string `toml:"interfaces"`
}

// ProcessMetricsConfig controls GET /integrations/metrics/processes.
type ProcessMetricsConfig struct {
	Enabled bool `toml:"enabled"`

	// Limit caps the number of processes returned, sorted by CPU usage desc.
	// Must be a positive integer, maximum 100.
	// Uses a pointer so we can distinguish "omitted" (nil → default 20)
	// from "explicitly set to 0" (→ validation error).
	Limit *int `toml:"limit"`
}

// PortMetricsConfig controls GET /integrations/metrics/ports.
type PortMetricsConfig struct {
	Enabled bool `toml:"enabled"`
}
