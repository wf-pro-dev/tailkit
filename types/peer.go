package types

import (
	"time"

	"tailscale.com/ipn/ipnstate"
)

// Peer is a tailnet machine identity as reported by Tailscale.
type Peer struct {
	ID        string            `json:"id,omitempty"`
	PublicKey string            `json:"public_key,omitempty"`
	HostName  string            `json:"host_name,omitempty"`
	DNSName   string            `json:"dns_name,omitempty"`
	IPs       []string          `json:"ips"`
	OS        string            `json:"os,omitempty"`
	Online    bool              `json:"online"`
	Metadata  map[string]string `json:"metadata,omitempty"`

	// Status keeps the raw Tailscale peer payload available for local SDK
	// enrichment and classification without making it part of the shared wire
	// contract.
	Status *ipnstate.PeerStatus `json:"-"`
}

// Host is a tailnet peer that has an associated tailkitd sidecar.
type Host struct {
	Name         string            `json:"name"`
	Role         string            `json:"role"`
	Environment  string            `json:"environment"`
	Provider     string            `json:"provider"`
	InstanceType string            `json:"instance_type"`
	Tags         []string          `json:"tags"`
	Metadata     map[string]string `json:"metadata"`

	Peer     *Peer     `json:"peer,omitempty"`
	Tailkitd *Tailkitd `json:"tailkitd,omitempty"`

	TSHostname string   `json:"ts_hostname"`
	TSDNSName  string   `json:"ts_dns_name"`
	TSIPs      []string `json:"ts_ips"`
	OS         string   `json:"os"`
	Arch       string   `json:"arch"`
	Online     bool     `json:"online"`
	IsAdmin    bool     `json:"is_admin"`
}

// IsClassified reports whether an admin has assigned a non-default role.
func (h *Host) IsClassified() bool {
	return h != nil && h.Role != "" && h.Role != "unclassified"
}

// ServiceCapabilities describes which portable service operations are exposed.
type ServiceCapabilities struct {
	Status  bool `json:"status"`
	Start   bool `json:"start"`
	Stop    bool `json:"stop"`
	Restart bool `json:"restart"`
	Reload  bool `json:"reload"`
	Logs    bool `json:"logs"`
}

// ServiceStatus is the current runtime state of a service-like workload.
type ServiceStatus struct {
	State     string     `json:"state,omitempty"`
	Health    string     `json:"health,omitempty"`
	Message   string     `json:"message,omitempty"`
	PID       int        `json:"pid,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Service represents a workload declared on a tailkitd node.
type Service struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Runtime     string `json:"runtime"`
	HostName    string `json:"host_name,omitempty"`
	NodeName    string `json:"node_name,omitempty"`
	Description string `json:"description,omitempty"`

	Source        string   `json:"source,omitempty"`
	Priority      string   `json:"priority,omitempty"`
	Tags          []string `json:"tags"`
	Version       string   `json:"version,omitempty"`
	Addresses     []string `json:"addresses"`
	Ports         []uint16 `json:"ports"`
	ExpectedPorts []uint16 `json:"expected_ports,omitempty"`

	SystemdUnit string `json:"systemd_unit,omitempty"`
	ContainerID string `json:"container_id,omitempty"`
	BinaryPath  string `json:"binary_path,omitempty"`
	PidFile     string `json:"pid_file,omitempty"`

	Capabilities ServiceCapabilities `json:"capabilities"`
	Status       ServiceStatus       `json:"status"`
}

// Normalize fills nil collections and backwards-compatible host fields.
func (s *Service) Normalize() {
	if s == nil {
		return
	}
	if s.Tags == nil {
		s.Tags = []string{}
	}
	if s.Addresses == nil {
		s.Addresses = []string{}
	}
	if s.Ports == nil {
		s.Ports = []uint16{}
	}
	if s.ExpectedPorts == nil {
		s.ExpectedPorts = []uint16{}
	}
	if s.HostName == "" {
		s.HostName = s.NodeName
	}
	if s.NodeName == "" {
		s.NodeName = s.HostName
	}
}

// Tailkitd is a tailkitd-* tsnet sidecar peer that manages one host.
type Tailkitd struct {
	HostName string    `json:"host_name,omitempty"`
	Peer     *Peer     `json:"peer,omitempty"`
	Services []Service `json:"services"`
}
