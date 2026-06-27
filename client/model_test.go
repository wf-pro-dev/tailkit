package client

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"
)

func TestCoreModelsJSONRoundTrip(t *testing.T) {
	startedAt := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	updatedAt := startedAt.Add(time.Minute)

	model := Host{
		Name:        "node-01",
		Role:        "worker",
		Environment: "prod",
		Tags:        []string{"docker"},
		Metadata:    map[string]string{"region": "eu"},
		Peer: &Peer{
			PublicKey: "nodekey:abc",
			HostName:  "node-01",
			DNSName:   "node-01.tailnet.ts.net.",
			IPs:       []string{"100.64.0.1"},
			OS:        "linux",
			Online:    true,
		},
		Tailkitd: &Tailkitd{
			HostName: "node-01",
			Peer: &Peer{
				PublicKey: "nodekey:def",
				HostName:  "tailkitd-node-01",
				DNSName:   "tailkitd-node-01.tailnet.ts.net.",
				IPs:       []string{"100.64.0.2"},
				OS:        "linux",
				Online:    true,
			},
			Services: []Service{{
				ID:        "systemd:sshd.service",
				Name:      "sshd",
				Kind:      "systemd",
				Runtime:   "systemd",
				HostName:  "node-01",
				Addresses: []string{"http://node-01.tailnet.ts.net"},
				Ports:     []uint16{22},
				Capabilities: ServiceCapabilities{
					Status:  true,
					Start:   true,
					Stop:    true,
					Restart: true,
					Logs:    true,
				},
				Status: ServiceStatus{
					State:     "running",
					Health:    "healthy",
					PID:       123,
					StartedAt: &startedAt,
					UpdatedAt: &updatedAt,
				},
			}},
		},
	}

	data, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal model: %v", err)
	}

	var got Host
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}
	if got.Name != model.Name || got.Peer.HostName != model.Peer.HostName {
		t.Fatalf("unexpected host round trip: %#v", got)
	}
	if len(got.Tailkitd.Services) != 1 {
		t.Fatalf("expected one service, got %d", len(got.Tailkitd.Services))
	}
	if !got.Tailkitd.Services[0].Capabilities.Restart {
		t.Fatal("expected restart capability to survive round trip")
	}
}

func TestServiceNormalizeDefaults(t *testing.T) {
	svc := Service{NodeName: "node-01"}
	svc.Normalize()

	if svc.HostName != "node-01" {
		t.Fatalf("expected host name from node name, got %q", svc.HostName)
	}
	if svc.Tags == nil || svc.Addresses == nil || svc.Ports == nil || svc.ExpectedPorts == nil {
		t.Fatalf("expected nil slices to be normalized: %#v", svc)
	}
	if svc.Capabilities.Start || svc.Capabilities.Stop || svc.Capabilities.Restart || svc.Capabilities.Reload || svc.Capabilities.Logs || svc.Capabilities.Status {
		t.Fatalf("expected zero-value capabilities to default false: %#v", svc.Capabilities)
	}
}

func TestPeerRawStatusIsNotSerialized(t *testing.T) {
	peer := Peer{
		HostName: "node-01",
		IPs:      []string{},
		Status:   &ipnstate.PeerStatus{HostName: "raw-node-01"},
	}

	data, err := json.Marshal(peer)
	if err != nil {
		t.Fatalf("marshal peer: %v", err)
	}
	if strings.Contains(string(data), "raw-node-01") || strings.Contains(string(data), "status") {
		t.Fatalf("expected raw status to stay out of JSON, got %s", data)
	}
}

func TestServiceTailkitdGraphIsAcyclic(t *testing.T) {
	model := Tailkitd{
		HostName: "node-01",
		Services: []Service{{
			Name:    "dockerd",
			Runtime: "systemd",
		}},
	}
	if _, err := json.Marshal(model); err != nil {
		t.Fatalf("expected acyclic tailkitd model to marshal: %v", err)
	}
}
