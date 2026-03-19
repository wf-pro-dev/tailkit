package tailkit

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
)

// ExecWith injects vars into the environment of a local subprocess and runs it.
// Vars are set as KEY=VALUE environment variables. The subprocess inherits the
// current process's environment with the vars overlaid on top.
//
// Secrets exist only in the child process environment and disappear when it
// exits — they are never written to disk.
//
// Example:
//
//	vars, err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").List(ctx)
//	err = tailkit.ExecWith(ctx, vars, []string{"/usr/bin/node", "server.js"})
func ExecWith(ctx context.Context, vars map[string]string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("tailkit: ExecWith: argv must not be empty")
	}

	cmd := osexec.CommandContext(ctx, argv[0], argv[1:]...)

	// Start with the current process environment.
	env := os.Environ()

	// Build a set of keys already overridden by vars so we can skip duplicates
	// from the parent environment.
	override := make(map[string]bool, len(vars))
	for k := range vars {
		override[strings.ToUpper(k)] = true
	}

	// Filter parent env to exclude any keys that vars will override.
	filtered := env[:0]
	for _, kv := range env {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			filtered = append(filtered, kv)
			continue
		}
		key := strings.ToUpper(kv[:idx])
		if !override[key] {
			filtered = append(filtered, kv)
		}
	}

	// Append the vars as KEY=VALUE entries.
	for k, v := range vars {
		filtered = append(filtered, k+"="+v)
	}
	cmd.Env = filtered

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
