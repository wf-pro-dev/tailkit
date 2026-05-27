package tailkit

// Service represents a workload declared on a tailkitd node.
type Service struct {
	NodeName string `json:"node_name"`

	Name          string   `json:"name"`
	Source        string   `json:"source"`
	Runtime       string   `json:"runtime"`
	Priority      string   `json:"priority"`
	Tags          []string `json:"tags"`
	ExpectedPorts []uint16 `json:"expected_ports"`

	SystemdUnit string `json:"systemd_unit,omitempty"`
	BinaryPath  string `json:"binary_path,omitempty"`
	PidFile     string `json:"pid_file,omitempty"`

	ToolVersion string `json:"tool_version,omitempty"`
	ToolTsHost  string `json:"tool_ts_host,omitempty"`
}
