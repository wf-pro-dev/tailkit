package tailkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// HostConfig is the client payload for /admin/hosts/:hostname/config.
type HostConfig struct {
	Name         string            `json:"name"`
	Role         string            `json:"role"`
	Environment  string            `json:"environment"`
	Provider     string            `json:"provider"`
	InstanceType string            `json:"instance_type"`
	Tags         []string          `json:"tags"`
	Metadata     map[string]string `json:"metadata"`
}

// OutsiderServiceConfig is the client payload for /admin/hosts/:hostname/services/:name.
type OutsiderServiceConfig struct {
	Name          string   `json:"name,omitempty"`
	Runtime       string   `json:"runtime"`
	Priority      string   `json:"priority"`
	Tags          []string `json:"tags"`
	ExpectedPorts []uint16 `json:"expected_ports"`
	SystemdUnit   string   `json:"systemd_unit,omitempty"`
	BinaryPath    string   `json:"binary_path,omitempty"`
	PidFile       string   `json:"pid_file,omitempty"`
}

// AdminClient provides typed administrative access to one tailkitd node.
type AdminClient struct {
	node *NodeClient
	key  string
}

// Admin scopes an admin client to this node and admin key.
func (n *NodeClient) Admin(adminKey string) *AdminClient {
	return &AdminClient{node: n, key: adminKey}
}

func (a *AdminClient) adminDoPost(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("tailkit: marshal admin payload: %w", err)
	}
	return a.node.doWithHeaders(ctx, http.MethodPost, path, bytes.NewReader(body), nil, func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", a.key)
	})
}

func (a *AdminClient) adminDoDelete(ctx context.Context, path string) error {
	return a.node.doWithHeaders(ctx, http.MethodDelete, path, nil, nil, func(req *http.Request) {
		req.Header.Set("X-Admin-Key", a.key)
	})
}

func (a *AdminClient) PushHostConfig(ctx context.Context, cfg HostConfig) error {
	path := fmt.Sprintf("/admin/hosts/%s/config", url.PathEscape(a.node.Hostname()))
	if err := a.adminDoPost(ctx, path, cfg); err != nil {
		return fmt.Errorf("tailkit: push host config: %w", err)
	}
	return nil
}

func (a *AdminClient) PushService(ctx context.Context, name string, cfg OutsiderServiceConfig) error {
	path := fmt.Sprintf("/admin/hosts/%s/services/%s", url.PathEscape(a.node.Hostname()), url.PathEscape(name))
	if err := a.adminDoPost(ctx, path, cfg); err != nil {
		return fmt.Errorf("tailkit: push service: %w", err)
	}
	return nil
}

func (a *AdminClient) DeleteService(ctx context.Context, name string) error {
	path := fmt.Sprintf("/admin/hosts/%s/services/%s", url.PathEscape(a.node.Hostname()), url.PathEscape(name))
	if err := a.adminDoDelete(ctx, path); err != nil {
		return fmt.Errorf("tailkit: delete service: %w", err)
	}
	return nil
}

func (a *AdminClient) TransferTo(ctx context.Context, targetHostname string) error {
	if err := a.adminDoPost(ctx, "/admin/transfer", map[string]string{"target_host": targetHostname}); err != nil {
		return fmt.Errorf("tailkit: transfer admin: %w", err)
	}
	return nil
}
