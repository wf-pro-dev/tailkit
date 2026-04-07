package tailkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/swarm"
	"github.com/wf-pro-dev/tailkit/types"
	integrationsTypes "github.com/wf-pro-dev/tailkit/types/integrations"
	"tailscale.com/tsnet"
)

// ─── Node ─────────────────────────────────────────────────────────────────────

// Node returns a NodeClient that communicates with the tailkitd instance
// running on the named node. The hostname is the node's Tailscale hostname
// (e.g. "warehouse-13-1") — tailkit prepends "tailkitd-" to form the tsnet
// hostname "tailkitd-warehouse-13-1.<tailnet>.ts.net".
//
// Node construction is free — no network calls are made until a method is
// called on the returned client or one of its sub-clients.
func Node(srv *Server, hostname string) *NodeClient {
	ctx := context.Background()
	tailkitd, err := GetTailkitPeer(ctx, srv, hostname)
	if err != nil {
		return &NodeClient{srv: srv, tailkitd: nil}
	}
	return &NodeClient{srv: srv, tailkitd: tailkitd}
}

// NodeClient is the entry point for all operations on a single tailkitd node.
// Obtain one via tailkit.Node(srv, "hostname").
type NodeClient struct {
	srv      *Server
	tailkitd *types.TailkitPeer
}

// httpClient returns an *http.Client that routes connections through the
// caller's own tsnet server, enabling direct node-to-node communication
// without leaving the tailnet.
func (n *NodeClient) httpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: n.srv.Server.Dial,
		},
		Timeout: 60 * time.Second,
	}
}

// baseURL returns the base URL for the target node's tailkitd.
func (n *NodeClient) baseURL() string {
	return "http://" + n.tailkitd.Status.HostName
}

// do executes an HTTP request against the node and decodes the JSON response
// into out. If out is nil the response body is discarded.
func (n *NodeClient) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, n.baseURL()+path, body)
	if err != nil {
		return fmt.Errorf("tailkit: build request %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	resp, err := n.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("tailkit: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		// Decode hint from body if present.
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return mapAPIError(resp.StatusCode, path, errBody.Error)
	}
	if resp.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return mapAPIError(resp.StatusCode, path, errBody.Error)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("tailkit: decode response from %s: %w", path, err)
		}
	}
	return nil
}

// doRaw executes a request and returns the raw response body bytes.
func (n *NodeClient) doRaw(ctx context.Context, method, path string, body io.Reader, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, n.baseURL()+path, body)
	if err != nil {
		return nil, fmt.Errorf("tailkit: build request: %w", err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := n.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailkit: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &errBody)
		return nil, mapAPIError(resp.StatusCode, path, errBody.Error)
	}
	return data, nil
}

// mapAPIError converts an HTTP status and message into a typed tailkit error.
func mapAPIError(status int, path, msg string) error {
	switch status {
	case http.StatusServiceUnavailable:
		// Determine which integration is unavailable from the path.
		switch {
		case strings.Contains(path, "/docker"):
			return types.ErrDockerUnavailable
		case strings.Contains(path, "/systemd"):
			return types.ErrSystemdUnavailable
		case strings.Contains(path, "/metrics"):
			return types.ErrMetricsUnavailable
		case strings.Contains(path, "/files") || strings.Contains(path, "/receive"):
			return types.ErrReceiveNotConfigured
		case strings.Contains(path, "/vars"):
			return types.ErrVarScopeNotFound
		}
		return fmt.Errorf("tailkit: service unavailable: %s", msg)
	case http.StatusNotFound:
		if strings.Contains(path, "/tools") {
			return types.ErrToolNotFound
		}
		if strings.Contains(path, "/exec/") {
			return types.ErrCommandNotFound
		}
		return fmt.Errorf("tailkit: not found: %s", msg)
	case http.StatusForbidden:
		return types.ErrPermissionDenied
	default:
		return fmt.Errorf("tailkit: HTTP %d from %s: %s", status, path, msg)
	}
}

// ─── Tools ────────────────────────────────────────────────────────────────────

// Tools returns all tools registered on the node.
func (n *NodeClient) Tools(ctx context.Context) ([]types.Tool, error) {
	var tools []types.Tool
	if err := n.do(ctx, http.MethodGet, "/tools", nil, &tools); err != nil {
		return nil, err
	}
	return tools, nil
}

// HasTool reports whether the node has a specific tool installed at or above
// the given minimum version. An empty minVersion matches any version.
func (n *NodeClient) HasTool(ctx context.Context, name string, minVersion string) (bool, error) {
	tools, err := n.Tools(ctx)
	if err != nil {
		return false, err
	}
	for _, t := range tools {
		if t.Name != name {
			continue
		}
		if minVersion == "" {
			return true, nil
		}
		if versionAtLeast(t.Version, minVersion) {
			return true, nil
		}
	}
	return false, nil
}

// ─── Exec ─────────────────────────────────────────────────────────────────────

// ExecJob polls for the result of a previously submitted job.
func (n *NodeClient) ExecJob(ctx context.Context, jobID string) (types.JobResult, error) {
	var result types.JobResult
	if err := n.do(ctx, http.MethodGet, "/exec/jobs/"+url.PathEscape(jobID), nil, &result); err != nil {
		return types.JobResult{}, err
	}
	return result, nil
}

// ExecWait fires a command and blocks until it completes or ctx is cancelled.
// Cancelling ctx stops polling but does not cancel the running job on the node.
func (n *NodeClient) ExecWait(ctx context.Context, jobID string) (types.JobResult, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return types.JobResult{}, ctx.Err()
		case <-ticker.C:
			result, err := n.ExecJob(ctx, jobID)
			if err != nil {
				return types.JobResult{}, err
			}
			switch result.Status {
			case types.JobStatusCompleted, types.JobStatusFailed, types.JobStatusCancelled:
				return result, nil
			}
			// JobStatusAccepted or JobStatusRunning — keep polling.
		}
	}
}

// ─── Config ────────────────────────────────────────────────────────────────────

func (n *NodeClient) GetConfig(ctx context.Context, integration string, config any) error {
	if err := n.do(ctx, http.MethodGet, fmt.Sprintf("/%s/config", integration), nil, config); err != nil {
		return err
	}
	return nil
}

// ─── Files ────────────────────────────────────────────────────────────────────

// FilesClient provides typed access to the /files endpoints on a node.
// Obtain via NodeClient.Files().
type FilesClient struct {
	node *NodeClient
}

// Files returns a FilesClient for this node.
func (n *NodeClient) Files() *FilesClient {
	return &FilesClient{node: n}
}

func (fc *FilesClient) Config(ctx context.Context) (integrationsTypes.FilesConfig, error) {
	var config integrationsTypes.FilesConfig
	if err := fc.node.GetConfig(ctx, "files", &config); err != nil {
		return integrationsTypes.FilesConfig{}, err
	}
	return config, nil
}

// List returns the directory listing for path on the node.
func (fc *FilesClient) List(ctx context.Context, dirPath string) ([]types.DirEntry, error) {
	var entries []types.DirEntry
	if err := fc.node.do(ctx, http.MethodGet,
		"/files?dir="+url.QueryEscape(dirPath), nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// Read returns the content of a file on the node as a string.
func (fc *FilesClient) Read(ctx context.Context, path string) (string, error) {
	data, err := fc.node.doRaw(ctx, http.MethodGet,
		"/files?path="+url.QueryEscape(path), nil, "application/json")
	if err != nil {
		return "", err
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("tailkit: decode file content: %w", err)
	}
	return resp.Content, nil
}

func (fc *FilesClient) Stat(ctx context.Context, path string) (types.FileStat, error) {
	var stat types.FileStat
	if err := fc.node.do(ctx, http.MethodGet,
		"/files?path="+url.QueryEscape(path)+"&stat=true", nil, &stat); err != nil {
		return types.FileStat{}, err
	}
	return stat, nil
}

// Download fetches a file from the node and writes it to localPath.
func (fc *FilesClient) Download(ctx context.Context, remotePath, localPath string) error {
	data, err := fc.node.doRaw(ctx, http.MethodGet,
		"/files?path="+url.QueryEscape(remotePath), nil, "application/octet-stream")
	if err != nil {
		return err
	}
	return writeLocalFile(localPath, data)
}

// Send pushes a local file to the node. Returns a SendResult; if a post_recv
// hook was triggered, SendResult.JobID is set and can be polled with ExecJob.
func (fc *FilesClient) Send(ctx context.Context, req types.SendRequest) (types.SendResult, error) {

	failResult := types.SendResult{
		Filename:     req.Filename,
		ToolName:     req.ToolName,
		LocalPath:    req.LocalPath,
		DestMachine:  fc.node.tailkitd.Status.HostName,
		Success:      false,
		WrittenTo:    req.DestPath,
		BytesWritten: 0,
	}

	data, err := readLocalFile(req.LocalPath)
	if err != nil {
		failResult.Error = err.Error()
		return failResult, fmt.Errorf("tailkit: read %s: %w", req.LocalPath, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx,
		http.MethodPost, fc.node.baseURL()+"/files", bytes.NewReader(data))
	if err != nil {
		failResult.Error = err.Error()
		return failResult, err
	}
	if req.DestPath != "" {
		httpReq.Header.Set("X-Dest-Path", req.DestPath)
	} else {
		httpReq.Header.Set("X-Tool", req.ToolName)
		httpReq.Header.Set("X-Filename", req.Filename)
	}

	resp, err := fc.node.httpClient().Do(httpReq)
	if err != nil {
		failResult.Error = err.Error()
		return failResult, fmt.Errorf("tailkit: send %s: %w", req.LocalPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		failResult.Error = errBody.Error
		return failResult, mapAPIError(resp.StatusCode, "/files", errBody.Error)
	}

	var result types.SendResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		failResult.Error = err.Error()
		return failResult, fmt.Errorf("tailkit: decode send result: %w", err)
	}

	result.LocalPath = req.LocalPath
	result.DestMachine = fc.node.tailkitd.Status.HostName
	result.Success = true

	return result, nil
}

// SendDir pushes all files in a local directory to the node recursively.
// Returns one SendResult per file; errors are collected, not propagated.
func (fc *FilesClient) SendDir(ctx context.Context, req types.SendDirRequest) ([]types.SendResult, error) {
	files, err := walkDir(req.LocalDir)
	if err != nil {
		return nil, fmt.Errorf("tailkit: walk %s: %w", req.LocalDir, err)
	}

	var results []types.SendResult
	for _, localPath := range files {
		rel := strings.TrimPrefix(localPath, req.LocalDir)
		rel = strings.TrimPrefix(rel, "/")
		destPath := req.DestPath
		if !strings.HasSuffix(destPath, "/") {
			destPath += "/"
		}
		destPath += rel

		result, err := fc.Send(ctx, types.SendRequest{
			LocalPath: localPath,
			DestPath:  destPath,
		})
		if err != nil {
			// Collect error as a failed result; continue with remaining files.
			results = append(results, types.SendResult{WrittenTo: destPath})
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// ─── Vars ─────────────────────────────────────────────────────────────────────

// VarsClient provides typed access to the /vars endpoints on a node.
type VarsClient struct {
	node    *NodeClient
	project string
	env     string
}

// Vars returns a VarsClient scoped to project/env.
func (n *NodeClient) Vars(project, env string) *VarsClient {
	return &VarsClient{node: n, project: project, env: env}
}

func (vc *VarsClient) Config(ctx context.Context) (integrationsTypes.VarsConfig, error) {
	var config integrationsTypes.VarsConfig
	if err := vc.node.GetConfig(ctx, "vars", &config); err != nil {
		return integrationsTypes.VarsConfig{}, err
	}
	return config, nil
}

func (vc *VarsClient) scopePath() string {
	return "/vars/" + url.PathEscape(vc.project) + "/" + url.PathEscape(vc.env)
}

// List returns all vars in the scope as a map.
func (vc *VarsClient) List(ctx context.Context) (map[string]string, error) {
	var vars map[string]string
	if err := vc.node.do(ctx, http.MethodGet, vc.scopePath(), nil, &vars); err != nil {
		return nil, err
	}
	return vars, nil
}

// Get returns the value of a single var.
func (vc *VarsClient) Get(ctx context.Context, key string) (string, error) {
	var resp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	path := vc.scopePath() + "/" + url.PathEscape(key)
	if err := vc.node.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Value, nil
}

// Set writes a var to the scope.
func (vc *VarsClient) Set(ctx context.Context, key, value string) error {
	path := vc.scopePath() + "/" + url.PathEscape(key)
	return vc.node.do(ctx, http.MethodPut, path,
		strings.NewReader(value), nil)
}

// Delete removes a var from the scope.
func (vc *VarsClient) Delete(ctx context.Context, key string) error {
	path := vc.scopePath() + "/" + url.PathEscape(key)
	return vc.node.do(ctx, http.MethodDelete, path, nil, nil)
}

// Env returns all vars rendered as sorted KEY=VALUE lines suitable for
// sourcing in a shell script or writing to a .env file.
func (vc *VarsClient) Env(ctx context.Context) (string, error) {
	data, err := vc.node.doRaw(ctx, http.MethodGet,
		vc.scopePath()+"?format=env", nil, "")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ─── Docker ───────────────────────────────────────────────────────────────────

// DockerClient provides typed access to the /integrations/docker endpoints.
type DockerClient struct{ node *NodeClient }

func (n *NodeClient) Docker() *DockerClient { return &DockerClient{node: n} }

const dockerBase = "/integrations/docker"

// Available returns false if Docker is not configured or the daemon is down.
// Never returns a Go error — callers can use it as a boolean check.
func (dc *DockerClient) Available(ctx context.Context) (bool, error) {
	var resp map[string]bool
	if err := dc.node.do(ctx, http.MethodGet, dockerBase+"/available", nil, &resp); err != nil {
		return false, nil
	}
	return resp["available"], nil
}

func (dc *DockerClient) Config(ctx context.Context) (integrationsTypes.DockerConfig, error) {
	var config integrationsTypes.DockerConfig
	if err := dc.node.GetConfig(ctx, "integrations/docker", &config); err != nil {
		return integrationsTypes.DockerConfig{}, err
	}
	return config, nil
}

func (dc *DockerClient) Containers(ctx context.Context) ([]container.Summary, error) {
	var out []container.Summary
	return out, dc.node.do(ctx, http.MethodGet, dockerBase+"/containers", nil, &out)
}

func (dc *DockerClient) Container(ctx context.Context, id string) (container.InspectResponse, error) {
	var out container.InspectResponse
	return out, dc.node.do(ctx, http.MethodGet,
		dockerBase+"/containers/"+url.PathEscape(id), nil, &out)
}

func (dc *DockerClient) Logs(ctx context.Context, id string, tail int) (string, error) {
	var resp struct {
		Logs string `json:"logs"`
	}
	path := fmt.Sprintf("%s/containers/%s/logs?tail=%d",
		dockerBase, url.PathEscape(id), tail)
	return resp.Logs, dc.node.do(ctx, http.MethodGet, path, nil, &resp)
}

func (dc *DockerClient) Start(ctx context.Context, id string) (types.Job, error) {
	var job types.Job
	return job, dc.node.do(ctx, http.MethodPost,
		dockerBase+"/containers/"+url.PathEscape(id)+"/start", nil, &job)
}

func (dc *DockerClient) Stop(ctx context.Context, id string) (types.Job, error) {
	var job types.Job
	return job, dc.node.do(ctx, http.MethodPost,
		dockerBase+"/containers/"+url.PathEscape(id)+"/stop", nil, &job)
}

func (dc *DockerClient) Restart(ctx context.Context, id string) (types.Job, error) {
	var job types.Job
	return job, dc.node.do(ctx, http.MethodPost,
		dockerBase+"/containers/"+url.PathEscape(id)+"/restart", nil, &job)
}

func (dc *DockerClient) Remove(ctx context.Context, id string) (types.Job, error) {
	var job types.Job
	return job, dc.node.do(ctx, http.MethodDelete,
		dockerBase+"/containers/"+url.PathEscape(id), nil, &job)
}

func (dc *DockerClient) Images(ctx context.Context) ([]image.Summary, error) {
	var out []image.Summary
	return out, dc.node.do(ctx, http.MethodGet, dockerBase+"/images", nil, &out)
}

func (dc *DockerClient) Pull(ctx context.Context, ref string) (types.Job, error) {
	var job types.Job
	return job, dc.node.do(ctx, http.MethodPost,
		dockerBase+"/images/pull?ref="+url.QueryEscape(ref), nil, &job)
}

// ComposeClient provides access to Docker Compose operations.
type ComposeClient struct{ node *NodeClient }

func (dc *DockerClient) Compose() *ComposeClient { return &ComposeClient{node: dc.node} }

func (cc *ComposeClient) Projects(ctx context.Context) ([]types.ComposeService, error) {
	var out []types.ComposeService
	return out, cc.node.do(ctx, http.MethodGet,
		dockerBase+"/compose/projects", nil, &out)
}

func (cc *ComposeClient) Project(ctx context.Context, name string) (types.ComposeService, error) {
	var out types.ComposeService
	return out, cc.node.do(ctx, http.MethodGet,
		dockerBase+"/compose/"+url.PathEscape(name), nil, &out)
}

func (cc *ComposeClient) Up(ctx context.Context, name, composefile string) (types.Job, error) {
	var job types.Job
	path := dockerBase + "/compose/" + url.PathEscape(name) + "/up"
	if composefile != "" {
		path += "?file=" + url.QueryEscape(composefile)
	}
	return job, cc.node.do(ctx, http.MethodPost, path, nil, &job)
}

func (cc *ComposeClient) Down(ctx context.Context, name string) (types.Job, error) {
	var job types.Job
	return job, cc.node.do(ctx, http.MethodPost,
		dockerBase+"/compose/"+url.PathEscape(name)+"/down", nil, &job)
}

func (cc *ComposeClient) Pull(ctx context.Context, name string) (types.Job, error) {
	var job types.Job
	return job, cc.node.do(ctx, http.MethodPost,
		dockerBase+"/compose/"+url.PathEscape(name)+"/pull", nil, &job)
}

func (cc *ComposeClient) Restart(ctx context.Context, name string) (types.Job, error) {
	var job types.Job
	return job, cc.node.do(ctx, http.MethodPost,
		dockerBase+"/compose/"+url.PathEscape(name)+"/restart", nil, &job)
}

func (cc *ComposeClient) Build(ctx context.Context, name string) (types.Job, error) {
	var job types.Job
	return job, cc.node.do(ctx, http.MethodPost,
		dockerBase+"/compose/"+url.PathEscape(name)+"/build", nil, &job)
}

// SwarmClient provides access to Docker Swarm read operations.
type SwarmClient struct{ node *NodeClient }

func (dc *DockerClient) Swarm() *SwarmClient { return &SwarmClient{node: dc.node} }

func (sc *SwarmClient) Nodes(ctx context.Context) ([]swarm.Node, error) {
	var out []swarm.Node
	return out, sc.node.do(ctx, http.MethodGet, dockerBase+"/swarm/nodes", nil, &out)
}

func (sc *SwarmClient) Services(ctx context.Context) ([]swarm.Service, error) {
	var out []swarm.Service
	return out, sc.node.do(ctx, http.MethodGet, dockerBase+"/swarm/services", nil, &out)
}

// ─── Systemd ──────────────────────────────────────────────────────────────────

// SystemdClient provides typed access to the /integrations/systemd endpoints.
type SystemdClient struct{ node *NodeClient }

func (n *NodeClient) Systemd() *SystemdClient { return &SystemdClient{node: n} }

const systemdBase = "/integrations/systemd"

func (sc *SystemdClient) Available(ctx context.Context) (bool, error) {
	var resp map[string]bool
	if err := sc.node.do(ctx, http.MethodGet, systemdBase+"/available", nil, &resp); err != nil {
		return false, nil
	}
	return resp["available"], nil
}

func (sc *SystemdClient) Config(ctx context.Context) (integrationsTypes.SystemdConfig, error) {
	var config integrationsTypes.SystemdConfig
	if err := sc.node.GetConfig(ctx, "integrations/systemd", &config); err != nil {
		return integrationsTypes.SystemdConfig{}, err
	}
	return config, nil
}

func (sc *SystemdClient) Units(ctx context.Context) ([]dbus.UnitStatus, error) {
	var out []dbus.UnitStatus
	return out, sc.node.do(ctx, http.MethodGet, systemdBase+"/units", nil, &out)
}

func (sc *SystemdClient) Unit(ctx context.Context, unit string) (map[string]any, error) {
	var out map[string]any
	return out, sc.node.do(ctx, http.MethodGet,
		systemdBase+"/units/"+url.PathEscape(unit), nil, &out)
}

func (sc *SystemdClient) UnitFile(ctx context.Context, unit string) (string, error) {
	var resp struct {
		Content string `json:"content"`
	}
	err := sc.node.do(ctx, http.MethodGet,
		systemdBase+"/units/"+url.PathEscape(unit)+"/file", nil, &resp)
	return resp.Content, err
}

func (sc *SystemdClient) Start(ctx context.Context, unit string) (types.Job, error) {
	var job types.Job
	return job, sc.node.do(ctx, http.MethodPost,
		systemdBase+"/units/"+url.PathEscape(unit)+"/start", nil, &job)
}

func (sc *SystemdClient) Stop(ctx context.Context, unit string) (types.Job, error) {
	var job types.Job
	return job, sc.node.do(ctx, http.MethodPost,
		systemdBase+"/units/"+url.PathEscape(unit)+"/stop", nil, &job)
}

func (sc *SystemdClient) Restart(ctx context.Context, unit string) (types.Job, error) {
	var job types.Job
	return job, sc.node.do(ctx, http.MethodPost,
		systemdBase+"/units/"+url.PathEscape(unit)+"/restart", nil, &job)
}

func (sc *SystemdClient) Reload(ctx context.Context, unit string) (types.Job, error) {
	var job types.Job
	return job, sc.node.do(ctx, http.MethodPost,
		systemdBase+"/units/"+url.PathEscape(unit)+"/reload", nil, &job)
}

func (sc *SystemdClient) Enable(ctx context.Context, unit string) (types.Job, error) {
	var job types.Job
	return job, sc.node.do(ctx, http.MethodPost,
		systemdBase+"/units/"+url.PathEscape(unit)+"/enable", nil, &job)
}

func (sc *SystemdClient) Disable(ctx context.Context, unit string) (types.Job, error) {
	var job types.Job
	return job, sc.node.do(ctx, http.MethodPost,
		systemdBase+"/units/"+url.PathEscape(unit)+"/disable", nil, &job)
}

func (sc *SystemdClient) Journal(ctx context.Context, unit string, lines int) ([]map[string]any, error) {
	var out []map[string]any
	path := fmt.Sprintf("%s/units/%s/journal?lines=%d",
		systemdBase, url.PathEscape(unit), lines)
	return out, sc.node.do(ctx, http.MethodGet, path, nil, &out)
}

func (sc *SystemdClient) SystemJournal(ctx context.Context, lines int) ([]map[string]any, error) {
	var out []map[string]any
	path := fmt.Sprintf("%s/journal?lines=%d", systemdBase, lines)
	return out, sc.node.do(ctx, http.MethodGet, path, nil, &out)
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

// MetricsClient provides typed access to the /integrations/metrics endpoints.
type MetricsClient struct{ node *NodeClient }

func (n *NodeClient) Metrics() *MetricsClient { return &MetricsClient{node: n} }

const metricsBase = "/integrations/metrics"

func (mc *MetricsClient) Available(ctx context.Context) (bool, error) {
	var resp map[string]bool
	if err := mc.node.do(ctx, http.MethodGet, metricsBase+"/available", nil, &resp); err != nil {
		return false, nil
	}
	return resp["available"], nil
}

func (mc *MetricsClient) Config(ctx context.Context) (integrationsTypes.MetricsConfig, error) {
	var config integrationsTypes.MetricsConfig
	if err := mc.node.GetConfig(ctx, "integrations/metrics", &config); err != nil {
		return integrationsTypes.MetricsConfig{}, err
	}
	return config, nil
}

func (mc *MetricsClient) Host(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/host", nil, &out)
}

func (mc *MetricsClient) CPU(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/cpu", nil, &out)
}

func (mc *MetricsClient) Memory(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/memory", nil, &out)
}

func (mc *MetricsClient) Disk(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/disk", nil, &out)
}

func (mc *MetricsClient) Network(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/network", nil, &out)
}

func (mc *MetricsClient) Processes(ctx context.Context) ([]map[string]any, error) {
	var out []map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/processes", nil, &out)
}

func (mc *MetricsClient) All(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/all", nil, &out)
}

// ─── tsnet dial helper ────────────────────────────────────────────────────────

// Ensure tsnet import is used.
var _ *tsnet.Server
