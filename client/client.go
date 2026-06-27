package client

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/swarm"
	gopsutildisk "github.com/shirou/gopsutil/v4/disk"
	gopsutilnet "github.com/shirou/gopsutil/v4/net"

	integrations "github.com/wf-pro-dev/tailkit/client/integrations"
	"golang.org/x/sync/errgroup"
)

const fleetConcurrency = 100

// ==========================================
// Third-Party Type Aliases
// ==========================================

// Container aliases the Docker SDK container summary.
type Container = container.Summary

// Image aliases the Docker SDK image summary.
type Image = image.Summary

// SwarmStatus aliases the Docker SDK swarm info.
type SwarmStatus = swarm.Swarm

// SystemdUnit aliases the CoreOS D-Bus unit status.
type SystemdUnit = dbus.UnitStatus

type Disk = *gopsutildisk.UsageStat

type Network = gopsutilnet.IOCountersStat

// ==========================================
// Options Payloads
// ==========================================

type PeerListOptions struct{ Online bool }
type HostListOptions struct {
	Online      bool
	Tags        []string
	Environment string
}
type AgentListOptions struct {
	Online bool
}
type ServiceListOptions struct {
	Kind    string
	Runtime string
}

// ==========================================
// Internal Backend Contract
// ==========================================

// ClientBackend defines the underlying network transport layer.
// All concrete scopes will hold a pointer to this to execute requests.
type ClientBackend interface {
	// Do executes an HTTP request against a specific node and decodes JSON into 'out'
	Do(ctx context.Context, hostname, method, path string, body io.Reader, out any) error

	// DoRaw executes a request and returns the raw bytes (useful for files/logs)
	DoRaw(ctx context.Context, hostname, method, path string, body io.Reader, accept string) ([]byte, error)

	// Stream opens an SSE or continuous log stream
	Stream(ctx context.Context, hostname, method, path string) (io.ReadCloser, error)

	Peers(ctx context.Context, opts *PeerListOptions) ([]string, error)

	Hosts(ctx context.Context, opts *HostListOptions) ([]string, error)

	Agents(ctx context.Context, opts *AgentListOptions) ([]string, error)
}

// ==========================================
// Top-Level Tailnet Client (Mockable)
// ==========================================
type TailnetClient interface {
	// Plural Namespaces (Optional variadic targets for fleet fan-outs)
	Peers(hostnames ...string) PeersScope
	Hosts(hostnames ...string) HostsScope
	Agents(hostnames ...string) AgentsScope
	Services(serviceIDs ...string) ServicesScope

	// Singular Namespaces
	Peer(hostname string) PeerScope
	Host(hostname string) HostScope
	Agent(hostname string) AgentScope
	Service(serviceID string) ServiceScope
}

func FanOut[T any](ctx context.Context, hostnames []string, fn func(context.Context, string) (T, error)) (map[string]T, map[string]error) {
	results := make(map[string]T, len(hostnames))
	errs := make(map[string]error)

	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fleetConcurrency)

	for _, hostname := range hostnames {
		g.Go(func() error {
			val, err := fn(ctx, hostname)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[hostname] = err
			} else {
				results[hostname] = val
			}
			return nil // never propagate — collect per-node
		})
	}
	_ = g.Wait()
	return results, errs
}

// ==========================================
// Plural Namespaces
// ==========================================

type PeersScope struct {
	Backend   ClientBackend
	Hostnames []string
}

func (h PeersScope) List(ctx context.Context, opt PeerListOptions) (map[string]Peer, map[string]error) {

	hostnames := h.Hostnames
	if len(h.Hostnames) == 0 {
		peers, err := h.Backend.Peers(ctx, &opt)
		if err != nil {
		}
		hostnames = peers
	}

	return FanOut(ctx, hostnames, func(ctx context.Context, hostname string) (Peer, error) {
		var host Peer
		err := h.Backend.Do(ctx, hostname, "GET", "/host", nil, &host)
		return host, err
	})
}

type HostsScope struct {
	Backend   ClientBackend
	Hostnames []string
}

func (h HostsScope) List(ctx context.Context, opts ...HostListOptions) (map[string]Host, map[string]error) {
	return FanOut(ctx, h.Hostnames, func(ctx context.Context, hostname string) (Host, error) {
		var host Host
		err := h.Backend.Do(ctx, hostname, "GET", "/host", nil, &host)
		return host, err
	})
}

func (h HostsScope) Files() FleetFilesScope {
	return FleetFilesScope{Backend: h.Backend, Hostnames: h.Hostnames}
}

type FleetFilesScope struct {
	Backend   ClientBackend
	Hostnames []string
}

func (ff FleetFilesScope) Config(ctx context.Context) (map[string]integrations.FilesConfig, map[string]error) {
	return FanOut(ctx, ff.Hostnames, func(ctx context.Context, hostname string) (integrations.FilesConfig, error) {
		var config integrations.FilesConfig
		err := ff.Backend.Do(ctx, hostname, "GET", "/files/config", nil, &config)
		return config, err
	})
}

func (ff FleetFilesScope) List(ctx context.Context, path string) (map[string][]DirEntry, map[string]error) {
	return FanOut(ctx, ff.Hostnames, func(ctx context.Context, hostname string) ([]DirEntry, error) {
		var out []DirEntry
		target := fmt.Sprintf("/files?dir=%s", url.QueryEscape(path))
		err := ff.Backend.Do(ctx, hostname, "GET", target, nil, &out)
		return out, err
	})
}

func (ff FleetFilesScope) Stat(ctx context.Context, path string) (map[string]FileStat, map[string]error) {
	return FanOut(ctx, ff.Hostnames, func(ctx context.Context, hostname string) (FileStat, error) {
		var out FileStat
		target := fmt.Sprintf("/files?path=%s&stat=true", url.QueryEscape(path))
		err := ff.Backend.Do(ctx, hostname, "GET", target, nil, &out)
		return out, err
	})
}

func (ff FleetFilesScope) Send(ctx context.Context, req SendRequest) (map[string]SendResult, map[string]error) {
	return FanOut(ctx, ff.Hostnames, func(ctx context.Context, hostname string) (SendResult, error) {
		var out SendResult
		err := ff.Backend.Do(ctx, hostname, "POST", "/files", nil, &out) // Uses proper reader in real impl
		return out, err
	})
}

type FleetMetricsScope struct {
	Backend   ClientBackend
	Hostnames []string
}

func (fm FleetMetricsScope) Config(ctx context.Context) (map[string]integrations.MetricsConfig, map[string]error) {
	return FanOut(ctx, fm.Hostnames, func(ctx context.Context, hostname string) (integrations.MetricsConfig, error) {
		var config integrations.MetricsConfig
		err := fm.Backend.Do(ctx, hostname, "GET", "/integrations/metrics/config", nil, &config)
		return config, err
	})
}

func (fm FleetMetricsScope) CPU(ctx context.Context) (map[string]CPU, map[string]error) {
	return FanOut(ctx, fm.Hostnames, func(ctx context.Context, hostname string) (CPU, error) {
		var out CPU
		err := fm.Backend.Do(ctx, hostname, "GET", "/integrations/metrics/cpu", nil, &out)
		return out, err
	})
}

func (fm FleetMetricsScope) Memory(ctx context.Context) (map[string]Memory, map[string]error) {
	return FanOut(ctx, fm.Hostnames, func(ctx context.Context, hostname string) (Memory, error) {
		var out Memory
		err := fm.Backend.Do(ctx, hostname, "GET", "/integrations/metrics/memory", nil, &out)
		return out, err
	})
}

func (fm FleetMetricsScope) All(ctx context.Context) (map[string]Metrics, map[string]error) {
	return FanOut(ctx, fm.Hostnames, func(ctx context.Context, hostname string) (Metrics, error) {
		var out Metrics
		err := fm.Backend.Do(ctx, hostname, "GET", "/integrations/metrics/all", nil, &out)
		return out, err
	})
}

func (h HostsScope) Metrics() FleetMetricsScope {
	return FleetMetricsScope{Backend: h.Backend, Hostnames: h.Hostnames}
}

type FleetVarsScope struct {
	Backend   ClientBackend
	Hostnames []string
	Project   string
	Env       string
}

func (fv FleetVarsScope) scopePath() string {
	return fmt.Sprintf("/vars/%s/%s", url.PathEscape(fv.Project), url.PathEscape(fv.Env))
}

func (fv FleetVarsScope) Config(ctx context.Context) (map[string]integrations.VarsConfig, map[string]error) {
	return FanOut(ctx, fv.Hostnames, func(ctx context.Context, hostname string) (integrations.VarsConfig, error) {
		var config integrations.VarsConfig
		err := fv.Backend.Do(ctx, hostname, "GET", "/vars/config", nil, &config)
		return config, err
	})
}

func (fv FleetVarsScope) List(ctx context.Context) (map[string]map[string]string, map[string]error) {
	return FanOut(ctx, fv.Hostnames, func(ctx context.Context, hostname string) (map[string]string, error) {
		var out map[string]string
		err := fv.Backend.Do(ctx, hostname, "GET", fv.scopePath(), nil, &out)
		return out, err
	})
}

func (fv FleetVarsScope) Set(ctx context.Context, key, value string) map[string]error {
	_, errs := FanOut(ctx, fv.Hostnames, func(ctx context.Context, hostname string) (struct{}, error) {
		path := fmt.Sprintf("%s/%s", fv.scopePath(), url.PathEscape(key))
		// Important: strings.NewReader must be recreated for every request in the loop
		err := fv.Backend.Do(ctx, hostname, "PUT", path, strings.NewReader(value), nil)
		return struct{}{}, err
	})
	return errs
}

func (h HostsScope) Vars(project, env string) FleetVarsScope {
	return FleetVarsScope{Backend: h.Backend, Hostnames: h.Hostnames, Project: project, Env: env}
}

type AgentsScope struct {
	Backend   ClientBackend
	Hostnames []string
}

func (h AgentsScope) List(ctx context.Context, opts ...AgentListOptions) (map[string]Tailkitd, map[string]error) {
	return FanOut(ctx, h.Hostnames, func(ctx context.Context, hostname string) (Tailkitd, error) {
		var agent Tailkitd
		err := h.Backend.Do(ctx, hostname, "GET", "/agent", nil, &agent)
		return agent, err
	})
}

type ServicesScope struct {
	Backend   ClientBackend
	TargetIDs []string
}

func (s ServicesScope) List(ctx context.Context, opts ...AgentListOptions) (map[string][]Service, map[string]error) {

	return FanOut(ctx, s.TargetIDs, func(ctx context.Context, hostname string) ([]Service, error) {
		var services []Service
		err := s.Backend.Do(ctx, hostname, "GET", "/services", nil, &services)
		return services, err
	})
}

// ==========================================
// Concrete Scopes (Stack Allocated, Zero GC)
// ==========================================

type PeerScope struct {
	Backend  ClientBackend
	Hostname string
}

func (p PeerScope) Get(ctx context.Context) (*Peer, error) {
	var out Peer
	err := p.Backend.Do(ctx, p.Hostname, "GET", "/peer", nil, &out)
	return &out, err
}

func (p PeerScope) Host() HostScope {
	return HostScope{Backend: p.Backend, Hostname: p.Hostname}
}

type HostScope struct {
	Backend  ClientBackend
	Hostname string
}

func (h HostScope) Get(ctx context.Context) (Host, error) {
	var out Host
	err := h.Backend.Do(ctx, h.Hostname, "GET", "/host", nil, &out)
	return out, err
}

func (h HostScope) Agent() AgentScope   { return AgentScope{Backend: h.Backend, Hostname: h.Hostname} }
func (h HostScope) Docker() DockerScope { return DockerScope{Backend: h.Backend, Hostname: h.Hostname} }
func (h HostScope) Systemd() SystemdScope {
	return SystemdScope{Backend: h.Backend, Hostname: h.Hostname}
}
func (h HostScope) Files() FilesScope { return FilesScope{Backend: h.Backend, Hostname: h.Hostname} }
func (h HostScope) Metrics() MetricsScope {
	return MetricsScope{Backend: h.Backend, Hostname: h.Hostname}
}
func (h HostScope) Vars(project, env string) VarsScope {
	return VarsScope{Backend: h.Backend, Hostname: h.Hostname, Project: project, Env: env}
}

// ==========================================
// Integration Scopes (Concrete)
// ==========================================
type DockerScope struct {
	Backend  ClientBackend
	Hostname string
}

func (d DockerScope) Containers() ContainersNamespace {
	return ContainersNamespace{Backend: d.Backend, Hostname: d.Hostname}
}
func (d DockerScope) Images() ImagesNamespace {
	return ImagesNamespace{Backend: d.Backend, Hostname: d.Hostname}
}
func (d DockerScope) Swarm() SwarmScope { return SwarmScope{Backend: d.Backend, Hostname: d.Hostname} }
func (d DockerScope) Compose() ComposeScope {
	return ComposeScope{Backend: d.Backend, Hostname: d.Hostname}
}
func (d DockerScope) Container(id string) ContainerScope {
	return ContainerScope{Backend: d.Backend, Hostname: d.Hostname, ContainerID: id}
}

func (d DockerScope) Stream(ctx context.Context, fn func(Event[DockerEvent]) error) error {
	body, err := d.Backend.Stream(ctx, d.Hostname, "GET", "/integrations/docker/stream")
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventDockerAll}, fn)
}

// Containers
type ContainersNamespace struct {
	Backend  ClientBackend
	Hostname string
}

func (c ContainersNamespace) List(ctx context.Context) ([]Container, error) {
	var out []Container
	err := c.Backend.Do(ctx, c.Hostname, "GET", "/integrations/docker/containers", nil, &out)
	return out, err
}

// Images
type ImagesNamespace struct {
	Backend  ClientBackend
	Hostname string
}

func (i ImagesNamespace) List(ctx context.Context) ([]Image, error) {
	var out []Image
	err := i.Backend.Do(ctx, i.Hostname, "GET", "/integrations/docker/images", nil, &out)
	return out, err
}
func (i ImagesNamespace) Pull(ctx context.Context, ref string) (Image, error) {
	var out Image
	path := "/integrations/docker/images/pull?ref=" + url.QueryEscape(ref)
	err := i.Backend.Do(ctx, i.Hostname, "POST", path, nil, &out)
	return out, err
}
func (i ImagesNamespace) Push(ctx context.Context, ref string) (Image, error) {
	var out Image
	path := "/integrations/docker/images/push?ref=" + url.QueryEscape(ref)
	err := i.Backend.Do(ctx, i.Hostname, "POST", path, nil, &out)
	return out, err
}

// Container Actions
type ContainerScope struct {
	Backend     ClientBackend
	Hostname    string
	ContainerID string
}

func (c ContainerScope) Start(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/docker/containers/%s/start", url.PathEscape(c.ContainerID))
	return c.Backend.Do(ctx, c.Hostname, "POST", path, nil, nil)
}
func (c ContainerScope) Stop(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/docker/containers/%s/stop", url.PathEscape(c.ContainerID))
	return c.Backend.Do(ctx, c.Hostname, "POST", path, nil, nil)
}
func (c ContainerScope) Restart(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/docker/containers/%s/restart", url.PathEscape(c.ContainerID))
	return c.Backend.Do(ctx, c.Hostname, "POST", path, nil, nil)
}
func (c ContainerScope) Logs(ctx context.Context, tailLines int) (io.ReadCloser, error) {
	path := fmt.Sprintf("/integrations/docker/containers/%s/logs?tail=%d", url.PathEscape(c.ContainerID), tailLines)
	return c.Backend.Stream(ctx, c.Hostname, "GET", path)
}
func (c ContainerScope) Inspect(ctx context.Context) (container.InspectResponse, error) {
	var out container.InspectResponse
	path := fmt.Sprintf("/integrations/docker/containers/%s", url.PathEscape(c.ContainerID))
	err := c.Backend.Do(ctx, c.Hostname, "GET", path, nil, &out)
	return out, err
}
func (c ContainerScope) Remove(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/docker/containers/%s", url.PathEscape(c.ContainerID))
	return c.Backend.Do(ctx, c.Hostname, "DELETE", path, nil, nil)
}

// Stream logs for a specific container
func (c ContainerScope) StreamLogs(ctx context.Context, tail int, fn func(Event[LogLine]) error) error {
	path := fmt.Sprintf("/integrations/docker/containers/%s/logs?tail=%d&follow=true", url.PathEscape(c.ContainerID), tail)
	body, err := c.Backend.Stream(ctx, c.Hostname, "GET", path)
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventLogLine}, fn)
}

// Stream stats for a specific container
func (c ContainerScope) StreamStats(ctx context.Context, fn func(Event[container.StatsResponse]) error) error {
	path := fmt.Sprintf("/integrations/docker/containers/%s/stats", url.PathEscape(c.ContainerID))
	body, err := c.Backend.Stream(ctx, c.Hostname, "GET", path)
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventStatsSnapshot}, fn)
}

// Compose
type ComposeScope struct {
	Backend  ClientBackend
	Hostname string
}

func (c ComposeScope) Projects(ctx context.Context) ([]ComposeService, error) {
	var out []ComposeService
	err := c.Backend.Do(ctx, c.Hostname, "GET", "/integrations/docker/compose/projects", nil, &out)
	return out, err
}
func (c ComposeScope) Project(name string) ComposeProjectScope {
	return ComposeProjectScope{Backend: c.Backend, Hostname: c.Hostname, ProjectName: name}
}

type ComposeProjectScope struct {
	Backend     ClientBackend
	Hostname    string
	ProjectName string
}

func (c ComposeProjectScope) Get(ctx context.Context) (ComposeService, error) {
	var out ComposeService
	path := fmt.Sprintf("/integrations/docker/compose/%s", url.PathEscape(c.ProjectName))
	err := c.Backend.Do(ctx, c.Hostname, "GET", path, nil, &out)
	return out, err
}
func (c ComposeProjectScope) Up(ctx context.Context, composeFile string) (Job, error) {
	var out Job
	path := fmt.Sprintf("/integrations/docker/compose/%s/up", url.PathEscape(c.ProjectName))
	if composeFile != "" {
		path += "?file=" + url.QueryEscape(composeFile)
	}
	err := c.Backend.Do(ctx, c.Hostname, "POST", path, nil, &out)
	return out, err
}
func (c ComposeProjectScope) Down(ctx context.Context) (Job, error) {
	var out Job
	path := fmt.Sprintf("/integrations/docker/compose/%s/down", url.PathEscape(c.ProjectName))
	err := c.Backend.Do(ctx, c.Hostname, "POST", path, nil, &out)
	return out, err
}
func (c ComposeProjectScope) Pull(ctx context.Context) (Job, error) {
	var out Job
	path := fmt.Sprintf("/integrations/docker/compose/%s/pull", url.PathEscape(c.ProjectName))
	err := c.Backend.Do(ctx, c.Hostname, "POST", path, nil, &out)
	return out, err
}
func (c ComposeProjectScope) Restart(ctx context.Context) (Job, error) {
	var out Job
	path := fmt.Sprintf("/integrations/docker/compose/%s/restart", url.PathEscape(c.ProjectName))
	err := c.Backend.Do(ctx, c.Hostname, "POST", path, nil, &out)
	return out, err
}
func (c ComposeProjectScope) Build(ctx context.Context) (Job, error) {
	var out Job
	path := fmt.Sprintf("/integrations/docker/compose/%s/build", url.PathEscape(c.ProjectName))
	err := c.Backend.Do(ctx, c.Hostname, "POST", path, nil, &out)
	return out, err
}

// Swarm
type SwarmScope struct {
	Backend  ClientBackend
	Hostname string
}

func (s SwarmScope) Leave(ctx context.Context) error {
	return s.Backend.Do(ctx, s.Hostname, "POST", "/integrations/docker/swarm/leave", nil, nil)
}
func (s SwarmScope) Join(ctx context.Context, tokens []string) error {
	// Simplified assuming tokens can be passed as query or body; adjust as per actual tailkitd spec
	return s.Backend.Do(ctx, s.Hostname, "POST", "/integrations/docker/swarm/join", nil, nil)
}
func (s SwarmScope) Status(ctx context.Context) (SwarmStatus, error) {
	var out SwarmStatus
	err := s.Backend.Do(ctx, s.Hostname, "GET", "/integrations/docker/swarm", nil, &out)
	return out, err
}

// Systemd
type SystemdScope struct {
	Backend  ClientBackend
	Hostname string
}

func (s SystemdScope) Units() UnitsNamespace {
	return UnitsNamespace{Backend: s.Backend, Hostname: s.Hostname}
}
func (s SystemdScope) Unit(name string) UnitScope {
	return UnitScope{Backend: s.Backend, Hostname: s.Hostname, UnitName: name}
}
func (s SystemdScope) SystemJournal(ctx context.Context, lines int) ([]JournalEntry, error) {
	var out []JournalEntry
	path := fmt.Sprintf("/integrations/systemd/journal?lines=%d", lines)
	err := s.Backend.Do(ctx, s.Hostname, "GET", path, nil, &out)
	return out, err
}

type UnitsNamespace struct {
	Backend  ClientBackend
	Hostname string
}

func (u UnitsNamespace) List(ctx context.Context) ([]SystemdUnit, error) {
	var out []SystemdUnit
	err := u.Backend.Do(ctx, u.Hostname, "GET", "/integrations/systemd/units", nil, &out)
	return out, err
}

type UnitScope struct {
	Backend  ClientBackend
	Hostname string
	UnitName string
}

func (u UnitScope) Start(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/start", url.PathEscape(u.UnitName))
	return u.Backend.Do(ctx, u.Hostname, "POST", path, nil, nil)
}
func (u UnitScope) Stop(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/stop", url.PathEscape(u.UnitName))
	return u.Backend.Do(ctx, u.Hostname, "POST", path, nil, nil)
}
func (u UnitScope) Restart(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/restart", url.PathEscape(u.UnitName))
	return u.Backend.Do(ctx, u.Hostname, "POST", path, nil, nil)
}
func (u UnitScope) Reload(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/reload", url.PathEscape(u.UnitName))
	return u.Backend.Do(ctx, u.Hostname, "POST", path, nil, nil)
}
func (u UnitScope) Enable(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/enable", url.PathEscape(u.UnitName))
	return u.Backend.Do(ctx, u.Hostname, "POST", path, nil, nil)
}
func (u UnitScope) Disable(ctx context.Context) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/disable", url.PathEscape(u.UnitName))
	return u.Backend.Do(ctx, u.Hostname, "POST", path, nil, nil)
}
func (u UnitScope) Status(ctx context.Context) (SystemdUnit, error) {
	var out SystemdUnit
	path := fmt.Sprintf("/integrations/systemd/units/%s/status", url.PathEscape(u.UnitName))
	err := u.Backend.Do(ctx, u.Hostname, "GET", path, nil, &out)
	return out, err
}
func (u UnitScope) Get(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	path := fmt.Sprintf("/integrations/systemd/units/%s", url.PathEscape(u.UnitName))
	err := u.Backend.Do(ctx, u.Hostname, "GET", path, nil, &out)
	return out, err
}
func (u UnitScope) File(ctx context.Context) (string, error) {
	var resp struct {
		Content string `json:"content"`
	}
	path := fmt.Sprintf("/integrations/systemd/units/%s/file", url.PathEscape(u.UnitName))
	err := u.Backend.Do(ctx, u.Hostname, "GET", path, nil, &resp)
	return resp.Content, err
}
func (u UnitScope) Journal(ctx context.Context, lines int) ([]JournalEntry, error) {
	var out []JournalEntry
	path := fmt.Sprintf("/integrations/systemd/units/%s/journal?lines=%d", url.PathEscape(u.UnitName), lines)
	err := u.Backend.Do(ctx, u.Hostname, "GET", path, nil, &out)
	return out, err
}
func (s SystemdScope) StreamSystemJournal(ctx context.Context, lines int, fn func(Event[JournalEntry]) error) error {
	path := fmt.Sprintf("/integrations/systemd/journal?lines=%d&follow=true", lines)
	body, err := s.Backend.Stream(ctx, s.Hostname, "GET", path)
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventJournalEntry}, fn)
}

func (u UnitScope) StreamJournal(ctx context.Context, lines int, fn func(Event[JournalEntry]) error) error {
	path := fmt.Sprintf("/integrations/systemd/units/%s/journal?lines=%d&follow=true", url.PathEscape(u.UnitName), lines)
	body, err := u.Backend.Stream(ctx, u.Hostname, "GET", path)
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventJournalEntry}, fn)
}

// Files
type FilesScope struct {
	Backend  ClientBackend
	Hostname string
}

func (f FilesScope) Dir(path string) DirScope {
	return DirScope{Backend: f.Backend, Hostname: f.Hostname, Path: path}
}
func (f FilesScope) File(path string) FileScope {
	return FileScope{Backend: f.Backend, Hostname: f.Hostname, Path: path}
}

type DirScope struct {
	Backend  ClientBackend
	Hostname string
	Path     string
}

func (d DirScope) List(ctx context.Context) ([]DirEntry, error) {
	var out []DirEntry
	path := fmt.Sprintf("/files?dir=%s", url.QueryEscape(d.Path))
	err := d.Backend.Do(ctx, d.Hostname, "GET", path, nil, &out)
	return out, err
}
func (d DirScope) Send(ctx context.Context, req SendDirRequest) ([]SendResult, error) {
	// Detailed implementation typically requires a multipart loop or zip. Kept simple to map contract.
	return nil, fmt.Errorf("SendDir requires client-side aggregation")
}

type FileScope struct {
	Backend  ClientBackend
	Hostname string
	Path     string
}

func (f FileScope) Config(ctx context.Context) (integrations.FilesConfig, error) {
	path := fmt.Sprintf("/files/config")
	var config integrations.FilesConfig
	err := f.Backend.Do(ctx, f.Hostname, "GET", path, nil, &config)
	if err != nil {
		return integrations.FilesConfig{}, err
	}
	return config, nil
}

func (f FileScope) Read(ctx context.Context) (io.ReadCloser, error) {
	path := fmt.Sprintf("/files?path=%s", url.QueryEscape(f.Path))
	return f.Backend.Stream(ctx, f.Hostname, "GET", path)
}
func (f FileScope) Write(ctx context.Context, content io.Reader) error {
	path := fmt.Sprintf("/files?path=%s", url.QueryEscape(f.Path))
	return f.Backend.Do(ctx, f.Hostname, "PUT", path, content, nil)
}
func (f FileScope) Stat(ctx context.Context) (FileStat, error) {
	var out FileStat
	path := fmt.Sprintf("/files?path=%s&stat=true", url.QueryEscape(f.Path))
	err := f.Backend.Do(ctx, f.Hostname, "GET", path, nil, &out)
	return out, err
}
func (f FileScope) Delete(ctx context.Context) error {
	path := fmt.Sprintf("/files?path=%s", url.QueryEscape(f.Path))
	return f.Backend.Do(ctx, f.Hostname, "DELETE", path, nil, nil)
}
func (f FileScope) Download(ctx context.Context, localDestPath string) error {
	// Typically wraps Read + local os.Write. Stubbed here as per SDK pattern.
	return fmt.Errorf("Download requires local OS writing logic")
}
func (f FileScope) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	var out SendResult
	err := f.Backend.Do(ctx, f.Hostname, "POST", "/files", nil, &out) // Uses proper reader in real impl
	return out, err
}

// Metrics
type MetricsScope struct {
	Backend  ClientBackend
	Hostname string
}

func (m MetricsScope) CPU(ctx context.Context) (CPU, error) {
	var out CPU
	err := m.Backend.Do(ctx, m.Hostname, "GET", "/integrations/metrics/cpu", nil, &out)
	return out, err
}
func (m MetricsScope) Memory(ctx context.Context) (Memory, error) {
	var out Memory
	err := m.Backend.Do(ctx, m.Hostname, "GET", "/integrations/metrics/memory", nil, &out)
	return out, err
}
func (m MetricsScope) Processes(ctx context.Context) ([]Process, error) {
	var out []Process
	err := m.Backend.Do(ctx, m.Hostname, "GET", "/integrations/metrics/processes", nil, &out)
	return out, err
}
func (m MetricsScope) Disk(ctx context.Context) ([]Disk, error) {
	var out []Disk
	err := m.Backend.Do(ctx, m.Hostname, "GET", "/integrations/metrics/disk", nil, &out)
	return out, err
}
func (m MetricsScope) Network(ctx context.Context) ([]Network, error) {
	var out []Network
	err := m.Backend.Do(ctx, m.Hostname, "GET", "/integrations/metrics/network", nil, &out)
	return out, err
}

func (m MetricsScope) StreamCPU(ctx context.Context, fn func(Event[CPU]) error) error {
	body, err := m.Backend.Stream(ctx, m.Hostname, "GET", "/integrations/metrics/cpu/stream")
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventCPU}, fn)
}

func (m MetricsScope) StreamMemory(ctx context.Context, fn func(Event[Memory]) error) error {
	body, err := m.Backend.Stream(ctx, m.Hostname, "GET", "/integrations/metrics/memory/stream")
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventMemory}, fn)
}

func (m MetricsScope) StreamAll(ctx context.Context, fn func(Event[Metrics]) error) error {
	body, err := m.Backend.Stream(ctx, m.Hostname, "GET", "/integrations/metrics/all/stream")
	if err != nil {
		return err
	}
	return StreamEvents(ctx, body, []string{EventAll}, fn)
}

// Vars
type VarsScope struct {
	Backend  ClientBackend
	Hostname string
	Project  string
	Env      string
}

func (v VarsScope) scopePath() string {
	return fmt.Sprintf("/vars/%s/%s", url.PathEscape(v.Project), url.PathEscape(v.Env))
}

func (v VarsScope) List(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	err := v.Backend.Do(ctx, v.Hostname, "GET", v.scopePath(), nil, &out)
	return out, err
}
func (v VarsScope) Get(ctx context.Context, key string) (string, error) {
	var resp struct {
		Value string `json:"value"`
	}
	path := fmt.Sprintf("%s/%s", v.scopePath(), url.PathEscape(key))
	err := v.Backend.Do(ctx, v.Hostname, "GET", path, nil, &resp)
	return resp.Value, err
}
func (v VarsScope) Set(ctx context.Context, key, value string) error {
	path := fmt.Sprintf("%s/%s", v.scopePath(), url.PathEscape(key))
	return v.Backend.Do(ctx, v.Hostname, "PUT", path, strings.NewReader(value), nil)
}
func (v VarsScope) Delete(ctx context.Context, key string) error {
	path := fmt.Sprintf("%s/%s", v.scopePath(), url.PathEscape(key))
	return v.Backend.Do(ctx, v.Hostname, "DELETE", path, nil, nil)
}

// ==========================================
// Agent & Service Scopes (Concrete)
// ==========================================

type AgentScope struct {
	Backend  ClientBackend
	Hostname string
}

func (a AgentScope) Get(ctx context.Context) (Tailkitd, error) {
	var out Tailkitd
	err := a.Backend.Do(ctx, a.Hostname, "GET", "/agent", nil, &out)
	return out, err
}

// AgentServicesScope is the concrete struct for an agent's local workloads.
type AgentServicesScope struct {
	Backend  ClientBackend
	Hostname string
}

func (s AgentServicesScope) List(ctx context.Context, opts ...ServiceListOptions) ([]Service, error) {
	var out []Service
	// Note: We are using s.Hostname here to route directly to this specific agent
	err := s.Backend.Do(ctx, s.Hostname, "GET", "/services", nil, &out)
	return out, err
}

func (a AgentScope) Services() AgentServicesScope {
	return AgentServicesScope{Backend: a.Backend, Hostname: a.Hostname}
}

func (a AgentScope) Logs() LogsScope { return LogsScope{Backend: a.Backend, Hostname: a.Hostname} }
func (a AgentScope) Events() EventsScope {
	return EventsScope{Backend: a.Backend, Hostname: a.Hostname}
}

func (a AgentScope) Execute(ctx context.Context, cmd string, args []string) ([]byte, error) {
	// Typically passed via body payload in actual JSON req
	return a.Backend.DoRaw(ctx, a.Hostname, "POST", "/exec", nil, "application/json")
}
func (a AgentScope) Restart(ctx context.Context) error {
	return a.Backend.Do(ctx, a.Hostname, "POST", "/agent/restart", nil, nil)
}
func (a AgentScope) Update(ctx context.Context, version string) error {
	path := fmt.Sprintf("/agent/update?version=%s", url.QueryEscape(version))
	return a.Backend.Do(ctx, a.Hostname, "POST", path, nil, nil)
}

// Sub-Scopes
type LogsScope struct {
	Backend  ClientBackend
	Hostname string
}

func (l LogsScope) Stream(ctx context.Context) (io.ReadCloser, error) {
	return l.Backend.Stream(ctx, l.Hostname, "GET", "/agent/logs")
}

type EventsScope struct {
	Backend  ClientBackend
	Hostname string
}

func (e EventsScope) Stream(ctx context.Context) (io.ReadCloser, error) {
	return e.Backend.Stream(ctx, e.Hostname, "GET", "/agent/events")
}

type ServiceScope struct {
	Backend   ClientBackend
	ServiceID string
}

func (s ServiceScope) Get(ctx context.Context) (Service, error) {
	var out Service
	path := fmt.Sprintf("/services/%s", url.PathEscape(s.ServiceID))
	err := s.Backend.Do(ctx, "", "GET", path, nil, &out) // Uses empty hostname to route globally by ID
	return out, err
}

func (s ServiceScope) Host() HostScope {
	return HostScope{Backend: s.Backend, Hostname: ""}
}

func (s ServiceScope) Start(ctx context.Context) error {
	path := fmt.Sprintf("/services/%s/start", url.PathEscape(s.ServiceID))
	return s.Backend.Do(ctx, "", "POST", path, nil, nil)
}
func (s ServiceScope) Stop(ctx context.Context) error {
	path := fmt.Sprintf("/services/%s/stop", url.PathEscape(s.ServiceID))
	return s.Backend.Do(ctx, "", "POST", path, nil, nil)
}
func (s ServiceScope) Restart(ctx context.Context) error {
	path := fmt.Sprintf("/services/%s/restart", url.PathEscape(s.ServiceID))
	return s.Backend.Do(ctx, "", "POST", path, nil, nil)
}
func (s ServiceScope) Reload(ctx context.Context) error {
	path := fmt.Sprintf("/services/%s/reload", url.PathEscape(s.ServiceID))
	return s.Backend.Do(ctx, "", "POST", path, nil, nil)
}
func (s ServiceScope) Logs(ctx context.Context, tailLines int) (io.ReadCloser, error) {
	path := fmt.Sprintf("/services/%s/logs?tail=%d", url.PathEscape(s.ServiceID), tailLines)
	return s.Backend.Stream(ctx, "", "GET", path)
}
