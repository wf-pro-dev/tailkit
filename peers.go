package tailkit

import (
	"context"
	"fmt"
	"strings"
)

// Discover finds all online tailnet peers that have the named tool installed.
// An empty minVersion matches any version.
func Discover(ctx context.Context, srv *Server, toolName string) ([]NodeInfo, error) {
	return DiscoverVersion(ctx, srv, toolName, "")
}

// DiscoverVersion is like Discover but requires at least minVersion.
func DiscoverVersion(ctx context.Context, srv *Server, toolName, minVersion string) ([]NodeInfo, error) {
	peers, err := onlinePeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	type result struct {
		info NodeInfo
		ok   bool
	}

	results := make(chan result, len(peers))
	sem := make(chan struct{}, 10)

	for _, peer := range peers {
		peer := peer
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			node := Node(srv, peer.hostname)
			tools, err := node.Tools(ctx)
			if err != nil {
				results <- result{}
				return
			}
			for _, t := range tools {
				if t.Name != toolName {
					continue
				}
				if minVersion != "" && !versionAtLeast(t.Version, minVersion) {
					continue
				}
				results <- result{
					info: NodeInfo{
						Name:        peer.hostname,
						TailscaleIP: peer.ip,
						Tool:        t,
					},
					ok: true,
				}
				return
			}
			results <- result{}
		}()
	}

	var found []NodeInfo
	for range peers {
		r := <-results
		if r.ok {
			found = append(found, r.info)
		}
	}
	return found, nil
}

// ─── peer discovery ───────────────────────────────────────────────────────────

type peerInfo struct {
	hostname string
	ip       string
}

// onlinePeers returns all online tailnet peers running tailkitd
// (hostname starts with "tailkitd-"), querying via the system Tailscale daemon.
func onlinePeers(ctx context.Context, srv *Server) ([]peerInfo, error) {
	lc := srv.localClient()
	if lc == nil {
		return nil, fmt.Errorf("tailkit: local client unavailable")
	}

	status, err := lc.Status(ctx)
	if err != nil {
		return nil, err
	}

	var peers []peerInfo
	for _, peer := range status.Peer {
		if !peer.Online {
			continue
		}
		name := peer.HostName
		if !strings.HasPrefix(name, "tailkitd-") {
			continue
		}
		nodeHostname := strings.TrimPrefix(name, "tailkitd-")
		ip := ""
		if len(peer.TailscaleIPs) > 0 {
			ip = peer.TailscaleIPs[0].String()
		}
		peers = append(peers, peerInfo{hostname: nodeHostname, ip: ip})
	}
	return peers, nil
}
