package types

// SystemdConfig is the parsed and validated representation of systemd.toml.
type SystemdConfig struct {
	Enabled bool
	Units   UnitConfig    `toml:"units"`
	Journal JournalConfig `toml:"journal"`
}

// JournalConfig controls journal retrieval behaviour.
// It applies to both per-unit journal endpoints and the system-wide journal.
type JournalConfig struct {
	// Enabled gates the per-unit journal endpoint
	// (GET /integrations/systemd/units/{unit}/journal).
	Enabled bool `toml:"enabled"`

	// Priority is the minimum log severity to return.
	// Valid values: emerg, alert, crit, err, warning, notice, info, debug.
	// Defaults to "info" if omitted.
	Priority string `toml:"priority"`

	// Lines is the default number of journal lines returned per request.
	// Must be a positive integer. Defaults to 100 if omitted.
	Lines int `toml:"lines"`

	// SystemJournal permits GET /integrations/systemd/journal (system-wide).
	// Kept as a dedicated bool because it is a distinct endpoint, not an
	// operation variant of the per-unit journal.
	SystemJournal bool `toml:"system_journal"`
}

// UnitConfig controls which systemd unit operations are permitted.
type UnitConfig struct {
	// Enabled gates all unit operations.
	Enabled bool `toml:"enabled"`

	// Allow is the list of permitted unit operations.
	// Valid values: list, inspect, unit_file, logs, start, stop, restart,
	// reload, enable, disable.
	// An unknown value is a fatal config error.
	Allow []string `toml:"allow"`
}
