package types

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
