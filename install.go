package tailkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
func Install(ctx context.Context, tool Tool) error {
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

func validateTool(t Tool) error {
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

	names := make(map[string]bool)
	for i, cmd := range t.Commands {
		if err := validateCommand(cmd, i); err != nil {
			return err
		}
		if names[cmd.Name] {
			return fmt.Errorf("Command[%d]: duplicate name %q", i, cmd.Name)
		}
		names[cmd.Name] = true
	}
	return nil
}

func validateCommand(cmd Command, idx int) error {
	prefix := fmt.Sprintf("Command[%d] %q", idx, cmd.Name)

	if cmd.Name == "" {
		return fmt.Errorf("Command[%d]: Name must not be empty", idx)
	}
	if !validName(cmd.Name) {
		return fmt.Errorf("%s: Name must match [a-zA-Z0-9_-]+", prefix)
	}
	if len(cmd.ExecParts) == 0 {
		return fmt.Errorf("%s: ExecParts must not be empty", prefix)
	}
	if cmd.ExecParts[0] == "" {
		return fmt.Errorf("%s: ExecParts[0] (binary path) must not be empty", prefix)
	}
	// Verify the binary exists on disk at install time.
	if _, err := os.Stat(cmd.ExecParts[0]); err != nil {
		return fmt.Errorf("%s: ExecParts[0] binary %q not found: %w", prefix, cmd.ExecParts[0], err)
	}
	if cmd.Timeout <= 0 {
		return fmt.Errorf("%s: Timeout must be a positive duration", prefix)
	}
	if cmd.ACLCap == "" {
		return fmt.Errorf("%s: ACLCap must not be empty", prefix)
	}

	argNames := make(map[string]bool)
	for j, arg := range cmd.Args {
		if err := validateArg(arg, idx, j); err != nil {
			return err
		}
		if argNames[arg.Name] {
			return fmt.Errorf("%s: Arg[%d]: duplicate name %q", prefix, j, arg.Name)
		}
		argNames[arg.Name] = true
	}

	// Verify that every {{.name}} slot in ExecParts refers to a declared arg.
	for _, part := range cmd.ExecParts {
		names := extractTemplateVars(part)
		for _, name := range names {
			if !argNames[name] {
				return fmt.Errorf("%s: ExecParts template variable {{.%s}} has no matching Arg declaration", prefix, name)
			}
		}
	}

	return nil
}

func validateArg(arg Arg, cmdIdx, argIdx int) error {
	prefix := fmt.Sprintf("Command[%d].Arg[%d] %q", cmdIdx, argIdx, arg.Name)

	if arg.Name == "" {
		return fmt.Errorf("Command[%d].Arg[%d]: Name must not be empty", cmdIdx, argIdx)
	}
	if !validName(arg.Name) {
		return fmt.Errorf("%s: Name must match [a-zA-Z0-9_-]+", prefix)
	}
	if arg.Type == "" {
		return fmt.Errorf("%s: Type must not be empty (use \"string\", \"int\", or \"bool\")", prefix)
	}
	switch arg.Type {
	case "string", "int", "bool":
		// valid
	default:
		return fmt.Errorf("%s: Type %q is not valid (use \"string\", \"int\", or \"bool\")", prefix, arg.Type)
	}
	if arg.Pattern != "" {
		if _, err := regexp.Compile(arg.Pattern); err != nil {
			return fmt.Errorf("%s: Pattern %q is not a valid regular expression: %w", prefix, arg.Pattern, err)
		}
	}
	return nil
}

// extractTemplateVars returns the variable names referenced in a text/template
// expression string, e.g. "{{.container}}" → ["container"].
// This is a simple parser sufficient for the subset of templates tailkit uses.
func extractTemplateVars(s string) []string {
	var vars []string
	for {
		start := strings.Index(s, "{{.")
		if start == -1 {
			break
		}
		s = s[start+3:]
		end := strings.Index(s, "}}")
		if end == -1 {
			break
		}
		name := strings.TrimSpace(s[:end])
		if name != "" {
			vars = append(vars, name)
		}
		s = s[end+2:]
	}
	return vars
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
