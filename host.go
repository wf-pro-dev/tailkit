package tailkit

// Host represents the unified identity of a tailkitd node,
// combining Tailscale network state with operator-declared metadata.
type Host struct {
	Name         string            `json:"name"`
	Role         string            `json:"role"`
	Environment  string            `json:"environment"`
	Provider     string            `json:"provider"`
	InstanceType string            `json:"instance_type"`
	Tags         []string          `json:"tags"`
	Metadata     map[string]string `json:"metadata"`

	TSHostname string   `json:"ts_hostname"`
	TSDNSName  string   `json:"ts_dns_name"`
	TSIPs      []string `json:"ts_ips"`
	OS         string   `json:"os"`
	Arch       string   `json:"arch"`
	Online     bool     `json:"online"`
	IsAdmin    bool     `json:"is_admin"`
}

// IsClassified reports whether an admin has assigned a non-default role.
func (h *Host) IsClassified() bool {
	return h != nil && h.Role != "" && h.Role != "unclassified"
}
