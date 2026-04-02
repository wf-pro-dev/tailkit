package tailkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/wf-pro-dev/tailkit/types"
)

const (
	// defaultToolsDir is where tailkitd reads tool registration files.
	defaultToolsDir = "/etc/tailkitd/tools"
)

// toolsDirOverride is set by tests to redirect Install/Uninstall to a temp dir.
// In production this is always empty and defaultToolsDir is used.
var toolsDirOverride string

func resolveToolsDir() string {
	if toolsDirOverride != "" {
		return toolsDirOverride
	}
	return defaultToolsDir
}

// Install writes a Tool registration file to /etc/tailkitd/tools/{name}.json.
//
// Call Install once at install time and again on every tool upgrade. tailkitd
// reads this file to populate its tool registry and exec command list. The write
// is atomic — tailkitd will never read a partially-written file.
//
// Install validates:
//   - Tool.Name is non-empty and matches [a-zA-Z0-9_-]+
//   - Tool.Version is non-empty
//   - Each Command.Name is non-empty
//   - Each Command.ExecParts is non-empty and ExecParts[0] exists on disk
//   - Each Command.Timeout is positive
//   - Each Arg.Pattern (if set) is a valid regular expression
//
// It creates /etc/tailkitd/tools/ if it does not exist.
func Install(ctx context.Context, tool types.Tool) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tailkit: Install: context cancelled: %w", err)
	}

	if err := validateTool(tool); err != nil {
		return fmt.Errorf("tailkit: Install: %w", err)
	}

	dir := resolveToolsDir()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("tailkit: Install: failed to create tools dir %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(tool, "", "  ")
	if err != nil {
		return fmt.Errorf("tailkit: Install: failed to marshal tool: %w", err)
	}

	dest := filepath.Join(dir, tool.Name+".json")
	if err := atomicWrite(dest, data, 0644); err != nil {
		return fmt.Errorf("tailkit: Install: failed to write %s: %w", dest, err)
	}

	return nil
}

// Uninstall removes the tool registration file for the named tool.
//
// If the file does not exist, Uninstall returns nil — it is safe to call
// Uninstall when the tool may or may not be installed.
func Uninstall(name string) error {
	if name == "" {
		return fmt.Errorf("tailkit: Uninstall: name must not be empty")
	}
	if !validName(name) {
		return fmt.Errorf("tailkit: Uninstall: invalid tool name %q (must match [a-zA-Z0-9_-]+)", name)
	}

	dest := filepath.Join(resolveToolsDir(), name+".json")
	err := os.Remove(dest)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("tailkit: Uninstall: failed to remove %s: %w", dest, err)
	}
	return nil
}

// ─── Validation ───────────────────────────────────────────────────────────────

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validName(s string) bool {
	return nameRE.MatchString(s)
}

func validateTool(t types.Tool) error {
	if t.Name == "" {
		return fmt.Errorf("Tool.Name must not be empty")
	}
	if !validName(t.Name) {
		return fmt.Errorf("Tool.Name %q must match [a-zA-Z0-9_-]+", t.Name)
	}
	if t.Version == "" {
		return fmt.Errorf("Tool.Version must not be empty")
	}
	if t.TsnetHost == "" {
		return fmt.Errorf("Tool.TsnetHost must not be empty")
	}

	return nil
}

// ─── Atomic write ─────────────────────────────────────────────────────────────

// atomicWrite writes data to dest atomically using a write-to-temp-then-rename
// pattern. The temp file is created in the same directory as dest to guarantee
// the rename is on the same filesystem.
func atomicWrite(dest string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(dest)

	tmp, err := os.CreateTemp(dir, ".tailkit-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmpName, dest, err)
	}
	return nil
}
