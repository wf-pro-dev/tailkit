package types

import (
	"path/filepath"
	"strings"
)

// FilesConfig is the parsed and validated representation of files.toml.
type FilesConfig struct {
	Enabled bool
	Paths   []PathRule `toml:"path"`
}

// PathRule defines access permissions for a single directory.
//
// Dir must be an absolute path ending with "/".
// Allow contains the permitted operations for that directory.
// WriteAsUser is the optional username to drop to when writing files.
// WriteAs is the resolved identity, populated at load time.
//
// When write_as is set but cannot be honoured (CAP_SETUID absent or username
// not found), a warning is logged at startup and the write proceeds as the
// daemon user — the write is NOT disabled.
type PathRule struct {
	// Dir is the directory this rule applies to.
	// Must be an absolute path ending with "/".
	Dir string `toml:"dir"`

	// Allow is the list of permitted operations for this directory.
	// Valid values: "read", "write".
	Allow []string `toml:"allow"`

	// WriteAsUser is the username to drop to when writing files to this path.
	// Requires the daemon to hold CAP_SETUID (granted by AmbientCapabilities
	// in the systemd unit). If absent, writes succeed as the daemon user.
	// Resolved to WriteAs at load time via os/user.Lookup.
	UseAsUser string `toml:"use_as"`

	// WriteAs is the resolved identity for WriteAsUser.
	// Zero value (Set=false) means no privilege drop — write as daemon user.
	// Populated by LoadFilesConfig; never set directly by callers.
	UseAs ResolvedIdentity `toml:"-"`
}

// ResolvedIdentity holds a uid/gid resolved from a username at startup.
type ResolvedIdentity struct {
	UID int
	GID int
	Set bool // true when a write_as user was successfully resolved
}

// matchReadRule finds the longest-prefix read rule covering path.
func (cfg *FilesConfig) MatchPathRule(path string) (PathRule, string, bool) {
	// For a file path, check its parent directory against the read rules.
	checkPath := path
	if !strings.HasSuffix(checkPath, "/") {
		checkPath = filepath.Dir(checkPath) + "/"
	}
	best := ""
	var bestRule PathRule
	for _, rule := range cfg.Paths {
		dir := rule.Dir
		if strings.HasPrefix(checkPath, dir) && len(dir) > len(best) {
			best = dir
			bestRule = rule
		}
	}
	if best == "" {
		return PathRule{}, "", false
	}
	return bestRule, best, true
}

func (r PathRule) Permits(op string) bool {
	for _, a := range r.Allow {
		if a == op {
			return true
		}
	}
	return false
}
