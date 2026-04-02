package types

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
	// Filename is the name of the file that was sent.
	Filename string `json:"filename"`
	// LocalPath is the absolute path to the file on the caller's machine.
	LocalPath string
	// DestPath is the absolute path the file should be written to on the node.
	DestPath string
}

// SendDirRequest describes a directory tree to push to a remote node.
type SendDirRequest struct {
	// ToolName is the name of the tool that sent the directory.
	ToolName string `json:"tool_name"`
	// Filename is the name of the file that was sent.
	Filename string `json:"filename"`
	// LocalDir is the absolute path to the source directory on the caller's machine.
	LocalDir string
	// DestPath is the absolute destination directory path on the node.
	DestPath string
}

// SendResult is the response from a Send or SendDir operation.
type SendResult struct {
	// ToolName is the name of the tool that sent the file.
	ToolName string `json:"tool_name"`

	// Filename is the name of the file that was sent.
	Filename string `json:"filename"`

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

// ---- Docker types ────────────────────────────────────────────────────

type ComposeService struct {
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	ConfigFiles []string `json:"config_files"`
}
