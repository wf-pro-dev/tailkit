package types

import (
	"context"
	"io"
)

// Option payloads for listing entities
type PeerListOptions struct{ OnlineOnly bool }
type HostListOptions struct {
	Tags        []string
	Environment string
}
type AgentListOptions struct{ ReachableOnly bool }
type ServiceListOptions struct {
	Kind    string
	Runtime string
}

type TailnetClient interface {
	// Plural Namespaces (Fleet-wide operations)
	Peers() PeersNamespace
	Hosts() HostsNamespace
	Agents() AgentsNamespace
	Services() ServicesNamespace

	// Singular Namespaces (Instance-scoped operations)
	Peer(hostname string) PeerEngine
	Host(hostname string) HostEngine
	Agent(hostname string) AgentEngine
	Service(serviceID string) ServiceEngine
}

type PeersNamespace interface {
	// List returns all raw Tailscale machines on the tailnet.
	List(ctx context.Context, opts ...PeerListOptions) ([]Peer, error)
}

type HostsNamespace interface {
	// List returns all operator-defined host identities
	List(ctx context.Context, opts ...HostListOptions) ([]Host, error)

	// Agents returns ALL agents currently running across the entire tailnet
	Agents(ctx context.Context, opts ...AgentListOptions) ([]Tailkitd, error)

	// Services returns ALL services running across all hosts
	Services(ctx context.Context, opts ...ServiceListOptions) ([]Service, error)
}

type ServicesNamespace interface {
	// List returns all active or cached workloads across the entire fleet (via Admin Cache).
	List(ctx context.Context, opts ...ServiceListOptions) ([]Service, error)
}

type AgentsNamespace interface {
	// List returns all tailkitd daemons running on the tailnet.
	List(ctx context.Context, opts ...AgentListOptions) ([]Tailkitd, error)
}

type PeerEngine interface {
	// Get retrieves the raw Tailscale metadata and presence status.
	Get(ctx context.Context) (*Peer, error)
}

type HostEngine interface {
	Get(ctx context.Context) (Host, error)

	// NEW: Explicitly traverse down the hierarchy to this host's specific agent!
	Agent() AgentEngine

	// Integration Engines
	Docker() DockerEngine
	Systemd() SystemdEngine
	Files() FilesEngine
	Metrics() MetricsEngine
	Vars() VarsEngine
}

type DockerEngine interface {
	Containers(ctx context.Context) ([]Container, error)
	Images(ctx context.Context) ([]Image, error)
	Swarm() SwarmEngine
}

type SwarmEngine interface {
	Leave(ctx context.Context) error
	Join(ctx context.Context, tokens []string) error
}

type SystemdEngine interface {
	Units(ctx context.Context) ([]SystemdUnit, error)
}

type FilesEngine interface {
	ListDir(ctx context.Context, path string) ([]FileInfo, error)
	ReadFile(ctx context.Context, path string) (io.ReadCloser, error)
}

type MetricsEngine interface {
	HostTelemetry(ctx context.Context) (NodeMetrics, error)
}

type VarsEngine interface {
	List(ctx context.Context) (map[string]string, error)
	Set(ctx context.Context, key, value string) error
}

// ==========================================
// Service-Scoped Engines
// ==========================================

// ServiceEngine maps to the capabilities defined in ServiceCapabilities
type ServiceEngine interface {
	Get() (Service, error) // Maps to 'Status' capability

	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Reload(ctx context.Context) error

	Logs(ctx context.Context, tailLines int) (io.ReadCloser, error)
}

type AgentEngine interface {
	Get(ctx context.Context) (Tailkitd, error)

	// NEW: Get only the services belonging to this specific agent/node
	Services(ctx context.Context) ([]Service, error)

	// Operational Actions
	Execute(ctx context.Context, cmd string, args []string) ([]byte, error)
	StreamLogs(ctx context.Context) (io.ReadCloser, error)
	Restart(ctx context.Context) error
}
