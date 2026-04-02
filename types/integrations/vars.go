package types

// VarsConfig is the parsed and validated representation of vars.toml.
type VarsConfig struct {
	Enabled bool
	Scopes  []VarScope `toml:"scope"`
}

// VarScope defines access permissions for a single project+env combination.
//
// Project and Env must both match ^[a-z0-9_-]+$.
// Allow must contain at least one of "read" or "write".
// Duplicate project/env pairs are a validation error.
type VarScope struct {
	// Project is the project identifier (e.g. "myapp").
	// Must match ^[a-z0-9_-]+$.
	Project string `toml:"project"`

	// Env is the environment identifier (e.g. "prod", "staging").
	// Must match ^[a-z0-9_-]+$.
	Env string `toml:"env"`

	// Allow is the list of permitted operations for this scope.
	// Valid values: "read", "write".
	// At least one value is required.
	Allow []string `toml:"allow"`
}
