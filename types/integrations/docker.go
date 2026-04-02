package types

// DockerConfig is the parsed and validated representation of docker.toml.
// Enabled is set to true only after a successful load — absent file means
// the docker integration is disabled (503), not an error.
type DockerConfig struct {
	Enabled    bool
	Containers DockerSectionConfig `toml:"containers"`
	Images     DockerSectionConfig `toml:"images"`
	Compose    DockerSectionConfig `toml:"compose"`
	Swarm      DockerSectionConfig `toml:"swarm"`
}

// DockerSectionConfig is the common shape for every docker.toml section.
// Enabled gates the entire section. Allow is the set of permitted operations
// within that section — validated at load time against the section's closed
// set of valid values.
type DockerSectionConfig struct {
	// Enabled gates all operations in this section.
	// If false, all endpoints in the section return 403 regardless of Allow.
	Enabled bool `toml:"enabled"`

	// Allow is the list of permitted operations within this section.
	// Valid values differ per section and are validated at startup.
	// An unknown value causes a fatal config error with the valid set listed.
	Allow []string `toml:"allow"`
}

// Permits returns true if op is both in the allow list and the section
// is enabled. Callers use this instead of inspecting Allow directly.
func (s DockerSectionConfig) Permits(op string) bool {
	if !s.Enabled {
		return false
	}
	for _, a := range s.Allow {
		if a == op {
			return true
		}
	}
	return false
}
