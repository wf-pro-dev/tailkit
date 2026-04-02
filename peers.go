package tailkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wf-pro-dev/tailkit/types"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

var (
	Peers       map[key.NodePublic]*ipnstate.PeerStatus
	lastUpdated time.Time
	TTL         = 15 * time.Minute
)

// Discover finds all online tailnet peers that have the named tool installed.
// An empty minVersion matches any version.
func Discover(ctx context.Context, srv *Server, toolName string) ([]types.Peer, error) {
	return DiscoverVersion(ctx, srv, toolName, "")
}

// DiscoverVersion is like Discover but requires at least minVersion.
func DiscoverVersion(ctx context.Context, srv *Server, toolName, minVersion string) ([]types.Peer, error) {
	peers, err := OnlinePeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	type result struct {
		info types.Peer
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

	var found []types.Peer
	for range peers {
		r := <-results
		if r.ok {
			found = append(found, r.info)
		}
	}
	return found, nil
}

// ─── peer discovery ───────────────────────────────────────────────────────────

func GetPeers(ctx context.Context, srv *Server) (map[key.NodePublic]*ipnstate.PeerStatus, error) {

	if time.Since(lastUpdated) < TTL && len(Peers) > 0 {
		return Peers, nil
	}

	lc := srv.localClient()
	if lc == nil {
		return nil, fmt.Errorf("tailkit: local client unavailable")
	}

	status, err := lc.Status(ctx)
	if err != nil {
		return nil, err
	}

	Peers = status.Peer
	lastUpdated = time.Now()

	return Peers, nil
}

func AllPeers(ctx context.Context, srv *Server) ([]types.Peer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	var allPeers []types.Peer
	for _, peer := range peers {

		if strings.HasPrefix(peer.HostName, "tailkitd-") {
			continue
		}

		currentPeer := types.Peer{
			Status:  *peer,
			Tailkit: nil,
		}

		tailkit, err := GetTailkitPeer(ctx, srv, currentPeer.Status.HostName)
		if err != nil {
			return nil, err
		}
		allPeers = append(allPeers, types.Peer{
			Status:  *peer,
			Tailkit: tailkit,
		})
	}
	return allPeers, nil
}

// OnlinePeers returns all online tailnet peers running tailkitd
// (hostname starts with "tailkitd-"), querying via the system Tailscale daemon.
func OnlinePeers(ctx context.Context, srv *Server) ([]types.Peer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	var onlinePeers []types.Peer
	for _, peer := range peers {
		if !peer.Online || strings.HasPrefix(peer.HostName, "tailkitd-") {
			continue
		}
		currentPeer := types.Peer{
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

func TailkitPeers(ctx context.Context, srv *Server) ([]types.TailkitPeer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	var tailkitPeers []types.TailkitPeer
	for _, peer := range peers {

		if !strings.HasPrefix(peer.HostName, "tailkitd-") {
			continue
		}

		tailkitPeer := types.TailkitPeer{
			Status: *peer,
			Tools:  []types.Tool{},
		}
		tailkitPeers = append(tailkitPeers, tailkitPeer)
	}
	return tailkitPeers, nil
}

func GetTailkitHostname(hostname string) string {
	return "tailkitd-" + hostname
}

func GetTailkitPeer(ctx context.Context, srv *Server, hostname string) (*types.TailkitPeer, error) {

	peers, err := GetPeers(ctx, srv)
	if err != nil {
		return nil, err
	}

	for _, p := range peers {
		if p.HostName == "tailkitd-"+hostname {
			return &types.TailkitPeer{
				Status: *p,
				Tools:  []types.Tool{},
			}, nil
		}
	}
	return nil, nil
}

func GetPeerTools(ctx context.Context, srv *Server, tailscaleIP string) ([]types.Tool, error) {
	return []types.Tool{}, nil
}
