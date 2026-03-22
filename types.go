package tailkit

import (
	"errors"
	"time"
)

// ─── Error sentinels ─────────────────────────────────────────────────────────

var (
	ErrReceiveNotConfigured = errors.New("tailkit: node has no files.toml (receive not configured)")
	ErrToolNotFound         = errors.New("tailkit: tool not installed on node")
	ErrCommandNotFound      = errors.New("tailkit: command not registered by tool")
	ErrDockerUnavailable    = errors.New("tailkit: node has no docker.toml or daemon not running")
	ErrSystemdUnavailable   = errors.New("tailkit: node has no systemd.toml or D-Bus unavailable")
	ErrMetricsUnavailable   = errors.New("tailkit: node has no metrics.toml")
	ErrVarScopeNotFound     = errors.New("tailkit: project/env scope not in vars.toml")
	ErrPermissionDenied     = errors.New("tailkit: ACL cap or node config blocked the operation")
)

// ─── Arg pattern constants ────────────────────────────────────────────────────

const (
	// PatternIdentifier matches container names, service names, etc.
	PatternIdentifier = `^[a-zA-Z0-9_-]+$`
	// PatternPath matches absolute unix paths.
	PatternPath = `^(/[a-zA-Z0-9_./-]+)+$`
	// PatternSemver matches semantic version strings.
	PatternSemver = `^v?[0-9]+\.[0-9]+\.[0-9]+$`
	// PatternIP matches IPv4 addresses.
	PatternIP = `^(\d{1,3}\.){3}\d{1,3}$`
	// PatternPort matches valid port numbers.
	PatternPort = `^([1-9][0-9]{0,4})$`
	// PatternFilename matches safe filenames.
	PatternFilename = `^[a-zA-Z0-9_.,-]+$`
)

// ─── Tool registration types ──────────────────────────────────────────────────

// Arg describes a single parameter accepted by a Command.
type Arg struct {
	// Name is the template variable name used in ExecParts (e.g. "container").
	Name string `json:"name"`
	// Type is the value type: "string", "int", "bool".
	Type string `json:"type"`
	// Required indicates the arg must be supplied by the caller.
	Required bool `json:"required"`
	// Pattern is a regex the value must match before substitution.
	// Use the PatternXxx constants or a custom expression.
	Pattern string `json:"pattern,omitempty"`
}

// Tool is the registration record written to /etc/tailkitd/tools/{name}.json
// by tailkit.Install and read by tailkitd to populate its tool registry.
type Tool struct {
	// Name is a unique identifier for the tool across the tailnet.
	Name string `json:"name"`
	// Version is the tool's current version string (semver recommended).
	Version string `json:"version"`
	// TsnetHost is the tsnet hostname this tool registers on the tailnet.
	TsnetHost string `json:"tsnet_host"`
}

// NodeInfo is returned by Discover — it identifies a tailnet peer that has a
// specific tool installed.
type NodeInfo struct {
	// Name is the Tailscale hostname of the node.
	Name string
	// TailscaleIP is the node's Tailscale IP address (100.x.x.x).
	TailscaleIP string
	// Tool is the matching Tool entry found on the node.
	Tool Tool
}

// ─── Job types ────────────────────────────────────────────────────────────────

// JobStatus represents the lifecycle state of an async job.
type JobStatus string

const (
	JobStatusAccepted  JobStatus = "accepted"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job is the immediate response from a fire-and-forget exec invocation.
type Job struct {
	// JobID is the opaque identifier used to poll for results.
	JobID  string    `json:"job_id"`
	Status JobStatus `json:"status"`
}

// JobResult is returned when polling a completed job.
type JobResult struct {
	JobID      string    `json:"job_id"`
	Status     JobStatus `json:"status"`
	ExitCode   int       `json:"exit_code"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	DurationMs int64     `json:"duration_ms"`
	// Error is set when the job failed to start (not when the command exits non-zero).
	Error string `json:"error,omitempty"`
}

// ─── File transfer types ──────────────────────────────────────────────────────

// SendRequest describes a single file to push to a remote node.
type SendRequest struct {
	// ToolName is the name of the tool that sent the file.
	ToolName string `json:"tool_name"`
	// LocalPath is the absolute path to the file on the caller's machine.
	LocalPath string
	// DestPath is the absolute path the file should be written to on the node.
	DestPath string
}

// SendDirRequest describes a directory tree to push to a remote node.
type SendDirRequest struct {
	// ToolName is the name of the tool that sent the directory.
	ToolName string `json:"tool_name"`
	// LocalDir is the absolute path to the source directory on the caller's machine.
	LocalDir string
	// DestPath is the absolute destination directory path on the node.
	DestPath string
}

// SendResult is the response from a Send or SendDir operation.
type SendResult struct {
	// ToolName is the name of the tool that sent the file.
	ToolName string `json:"tool_name"`

	LocalPath string `json:"local_path"`
	// Success indicates whether the file was successfully sent.
	Success bool `json:"success"`
	// WrittenTo is the absolute path the file was written to on the node.
	WrittenTo string `json:"written_to"`
	// BytesWritten is the number of bytes written.
	BytesWritten int64 `json:"bytes_written"`
	// DestMachine is the hostname of the machine the file was sent to.
	DestMachine string `json:"dest_machine"`
	//Error is set when the file was not successfully sent.
	Error string `json:"error,omitempty"`
}

// DirEntry is a single entry in a directory listing.
type DirEntry struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
	Mode    string    `json:"mode"`
}
