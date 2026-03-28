package tailkit

import (
	"context"
	"fmt"
	"strings"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

// Discover finds all online tailnet peers that have the named tool installed.
// An empty minVersion matches any version.
func Discover(ctx context.Context, srv *Server, toolName string) ([]Peer, error) {
	return DiscoverVersion(ctx, srv, toolName, "")
}

// DiscoverVersion is like Discover but requires at least minVersion.
func DiscoverVersion(ctx context.Context, srv *Server, toolName, minVersion string) ([]Peer, error) {
	peers, err := OnlinePeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	type result struct {
		info Peer
		ok   bool
	}

	results := make(chan result, len(peers))
	sem := make(chan struct{}, 10)

	for _, peer := range peers {
		peer := peer
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			node := Node(srv, peer.Status.HostName)
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
					info: peer,
					ok:   true,
				}
				return
			}
			results <- result{}
		}()
	}

	var found []Peer
	for range peers {
		r := <-results
		if r.ok {
			found = append(found, r.info)
		}
	}
	return found, nil
}

// ─── peer discovery ───────────────────────────────────────────────────────────

func AllPeers(ctx context.Context, srv *Server) ([]Peer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	var allPeers []Peer
	for _, peer := range peers {

		if strings.HasPrefix(peer.HostName, "tailkitd-") {
			continue
		}

		currentPeer := Peer{
			Status:  *peer,
			Tailkit: nil,
		}

		tailkit, err := GetTailkitPeer(ctx, srv, currentPeer.Status.HostName)
		if err != nil {
			return nil, err
		}
		allPeers = append(allPeers, Peer{
			Status:  *peer,
			Tailkit: tailkit,
		})
	}
	return allPeers, nil
}

func GetPeers(ctx context.Context, srv *Server) (map[key.NodePublic]*ipnstate.PeerStatus, error) {
	lc := srv.localClient()
	if lc == nil {
		return nil, fmt.Errorf("tailkit: local client unavailable")
	}

	status, err := lc.Status(ctx)
	if err != nil {
		return nil, err
	}

	return status.Peer, nil
}

// OnlinePeers returns all online tailnet peers running tailkitd
// (hostname starts with "tailkitd-"), querying via the system Tailscale daemon.
func OnlinePeers(ctx context.Context, srv *Server) ([]Peer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	var onlinePeers []Peer
	for _, peer := range peers {
		if !peer.Online || strings.HasPrefix(peer.HostName, "tailkitd-") {
			continue
		}
		currentPeer := Peer{
			Status: *peer,
		}
		tailkit, err := GetTailkitPeer(ctx, srv, currentPeer.Status.HostName)
		if err != nil {
			return nil, err
		}
		currentPeer.Tailkit = tailkit
		onlinePeers = append(onlinePeers, currentPeer)
	}
	return onlinePeers, nil
}

func TailkitPeers(ctx context.Context, srv *Server) ([]TailkitPeer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	var tailkitPeers []TailkitPeer
	for _, peer := range peers {

		if !strings.HasPrefix(peer.HostName, "tailkitd-") {
			continue
		}

		tailkitPeer := TailkitPeer{
			Status: *peer,
			Tools:  []Tool{},
		}
		tailkitPeers = append(tailkitPeers, tailkitPeer)
	}
	return tailkitPeers, nil
}

func GetTailkitPeer(ctx context.Context, srv *Server, hostname string) (*TailkitPeer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	for _, p := range peers {
		if p.Tags.ContainsFunc(func(t string) bool { return t == hostname }) {
			return &TailkitPeer{
				Status: *p,
				Tools:  []Tool{},
			}, nil
		}
	}
	return nil, nil
}

func GetPeerTools(ctx context.Context, srv *Server, tailscaleIP string) ([]Tool, error) {
	return []Tool{}, nil
}
