package tailkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wf-pro-dev/tailkit/client"
)

// ─── Top-Level Client Factory ─────────────────────────────────────────────────

// NewClient creates the entry point for all operations on the Tailnet.
// It initializes the concrete HTTP transport and returns the top-level namespaces.
func NewClient(srv *Server) client.TailnetClient {
	return &tailnetClient{
		backend: &httpBackend{srv: srv},
	}
}

// tailnetClient implements the top-level client.TailnetClient interface.
type tailnetClient struct {
	backend client.ClientBackend
}

// Singular Namespaces (Stack Allocated Scopes)
func (c *tailnetClient) Peer(hostname string) client.PeerScope {
	return client.PeerScope{Backend: c.backend, Hostname: hostname}
}
func (c *tailnetClient) Host(hostname string) client.HostScope {
	return client.HostScope{Backend: c.backend, Hostname: hostname}
}
func (c *tailnetClient) Agent(hostname string) client.AgentScope {
	return client.AgentScope{Backend: c.backend, Hostname: hostname}
}
func (c *tailnetClient) Service(serviceID string) client.ServiceScope {
	return client.ServiceScope{Backend: c.backend, ServiceID: serviceID}
}

// Plural Namespaces (Implementation for fleet-wide actions)
// Note: These would typically return concrete implementations that hit the Admin Node cache
// or execute the distributed fan-out (formerly in fleet.go).
func (c *tailnetClient) Peers(hostnames ...string) client.PeersScope {
	return client.PeersScope{Backend: c.backend, Hostnames: hostnames}
}

func (c *tailnetClient) Hosts(hostnames ...string) client.HostsScope {
	return client.HostsScope{Backend: c.backend, Hostnames: hostnames}
}

func (c *tailnetClient) Agents(hostnames ...string) client.AgentsScope {
	return client.AgentsScope{Backend: c.backend, Hostnames: hostnames}
}

func (c *tailnetClient) Services(serviceIDs ...string) client.ServicesScope {
	return client.ServicesScope{Backend: c.backend, TargetIDs: serviceIDs}
} // To be implemented

// ─── HTTP Backend Implementation ──────────────────────────────────────────────

// httpBackend implements client.ClientBackend, providing the actual network transport
// for the stack-allocated scopes.
type httpBackend struct {
	srv *Server
}

// tailkitdHostname returns the tsnet hostname for a host's tailkitd sidecar.
func tailkitdHostname(hostname string) string {
	return "tailkitd-" + hostname
}

// baseURL returns the base URL for the target node's tailkitd.
func (b *httpBackend) baseURL(hostname string) string {
	return "http://" + tailkitdHostname(hostname)
}

// Do executes an HTTP request against the node and decodes the JSON response into 'out'.
func (b *httpBackend) Do(ctx context.Context, hostname, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, b.baseURL(hostname)+path, body)
	if err != nil {
		return fmt.Errorf("tailkit: build request %s %s: %w", method, path, err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	resp, err := b.srv.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("tailkit: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return b.handleError(resp, path)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("tailkit: decode response from %s: %w", path, err)
		}
	}
	return nil
}

// DoRaw executes a request and returns the raw response body bytes.
// Ideal for file downloads or raw string endpoints.
func (b *httpBackend) DoRaw(ctx context.Context, hostname, method, path string, body io.Reader, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.baseURL(hostname)+path, body)
	if err != nil {
		return nil, fmt.Errorf("tailkit: build request: %w", err)
	}

	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := b.srv.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailkit: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, b.handleErrorWithBody(resp.StatusCode, data, path)
	}
	return data, nil
}

// Stream opens a long-lived HTTP connection for SSE or continuous logging.
func (b *httpBackend) Stream(ctx context.Context, hostname, method, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.baseURL(hostname)+path, nil)
	if err != nil {
		return nil, fmt.Errorf("tailkit: build stream request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")

	// Uses a specialized HTTP client configured for long-lived streams
	resp, err := b.srv.StreamHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailkit: stream %s: %w", path, err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, b.handleError(resp, path)
	}

	// Caller is responsible for closing the body
	return resp.Body, nil
}

// ─── Error Handling ───────────────────────────────────────────────────────────

// handleError extracts the JSON error from a response body and routes it to mapAPIError.
func (b *httpBackend) handleError(resp *http.Response, path string) error {
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	return mapAPIError(resp.StatusCode, path, errBody.Error)
}

// handleErrorWithBody operates on already-read bytes.
func (b *httpBackend) handleErrorWithBody(statusCode int, body []byte, path string) error {
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &errBody)
	return mapAPIError(statusCode, path, errBody.Error)
}

// mapAPIError converts an HTTP status and message into a typed tailkit error.
func mapAPIError(status int, path, msg string) error {
	switch status {
	case http.StatusServiceUnavailable:
		// Determine which integration is unavailable from the path.
		switch {
		case strings.Contains(path, "/docker"):
			return client.ErrDockerUnavailable
		case strings.Contains(path, "/systemd"):
			return client.ErrSystemdUnavailable
		case strings.Contains(path, "/metrics"):
			return client.ErrMetricsUnavailable
		case strings.Contains(path, "/files") || strings.Contains(path, "/receive"):
			return client.ErrReceiveNotConfigured
		case strings.Contains(path, "/vars"):
			return client.ErrVarScopeNotFound
		}
		return fmt.Errorf("tailkit: service unavailable: %s", msg)
	case http.StatusNotFound:
		if strings.Contains(path, "/tools") {
			return client.ErrToolNotFound
		}
		if strings.Contains(path, "/exec/") {
			return client.ErrCommandNotFound
		}
		return fmt.Errorf("tailkit: not found: %s", msg)
	case http.StatusForbidden:
		return client.ErrPermissionDenied
	case http.StatusUnauthorized:
		return client.ErrUnauthorized
	case http.StatusConflict:
		return client.ErrConflict
	default:
		return fmt.Errorf("tailkit: HTTP %d from %s: %s", status, path, msg)
	}
}

func (b *httpBackend) Peers(ctx context.Context, opt *client.PeerListOptions) ([]string, error) {

	ps, err := ListPeers(ctx, b.srv, opt)
	if err != nil {
		return nil, err
	}

	var peers = make([]string, len(ps))

	for _, p := range ps {
		peers = append(peers, p.HostName)
	}

	return peers, nil
}

func (b *httpBackend) Hosts(ctx context.Context, opt *client.HostListOptions) ([]string, error) {

	ps, err := ListHosts(ctx, b.srv, nil)
	if err != nil {
		return nil, err
	}

	var hosts = make([]string, len(ps))

	for _, h := range ps {
		hosts = append(hosts, h.Name)
	}

	return hosts, nil
}

func (b *httpBackend) Agents(ctx context.Context, opt *client.AgentListOptions) ([]string, error) {

	ps, err := ListAgents(ctx, b.srv, nil)
	if err != nil {
		return nil, err
	}

	var peers = make([]string, len(ps))

	for _, p := range ps {
		peers = append(peers, p.HostName)
	}

	return peers, nil
}
