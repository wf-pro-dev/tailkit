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

// ListOpts filters entity discovery results derived from local Tailscale status.
//
// Online semantics per entity:
//   - Peer: peer.Online (tailkitd-* sidecars are always excluded)
//   - Host: tailkitd sidecar is online and API-reachable
//   - Tailkitd: sidecar peer is online
type ListOpts struct {
	Online bool
}

var (
	// ListAll returns every known entry with no online filter.
	ListAll = ListOpts{}
	// ListOnline returns only online or reachable entries.
	ListOnline = ListOpts{Online: true}
)

type peerCacheEntry struct {
	peers     map[key.NodePublic]*ipnstate.PeerStatus
	view      peerClassification
	fetchedAt time.Time
}

// ListPeers returns tailnet machine peers from local Tailscale status.
// tailkitd-* sidecar peers are excluded.
func ListPeers(ctx context.Context, srv *Server, opts *ListOpts) ([]types.Peer, error) {

	if opts == nil {
		opts.Online = true
	}

	entry, err := getPeerCache(ctx, srv)
	if err != nil {
		return nil, err
	}
	peers := machinePeers(entry.view.peers)
	if !opts.Online {
		return peers, nil
	}
	online := make([]types.Peer, 0, len(peers))
	for _, peer := range peers {
		if peer.Online {
			online = append(online, peer)
		}
	}
	return online, nil
}

// ListHosts returns hosts classified from Tailscale status by pairing each
// machine peer with its tailkitd-<hostname> sidecar.
//
// Results do not include operator metadata from tailkitd's /host API.
// Use Node(...).Host or FleetClient.Hosts for that.
func ListHosts(ctx context.Context, srv *Server, opts ListOpts) ([]types.Host, error) {
	entry, err := getPeerCache(ctx, srv)
	if err != nil {
		return nil, err
	}
	hosts := hostsSlice(entry.view.hosts)
	if !opts.Online {
		return hosts, nil
	}
	online := make([]types.Host, 0, len(hosts))
	for _, host := range hosts {
		if hostReachable(host) {
			online = append(online, host)
		}
	}
	return online, nil
}

// ListTailkitds returns tailkitd-* sidecar peers, including orphans without a
// matching host peer.
func ListTailkitds(ctx context.Context, srv *Server, opts ListOpts) ([]types.Tailkitd, error) {
	entry, err := getPeerCache(ctx, srv)
	if err != nil {
		return nil, err
	}
	tailkitds := make([]types.Tailkitd, 0, len(entry.view.tailkitds))
	for _, tailkitd := range entry.view.tailkitds {
		tailkitds = append(tailkitds, *tailkitd)
	}
	if !opts.Online {
		return tailkitds, nil
	}
	online := make([]types.Tailkitd, 0, len(tailkitds))
	for _, tailkitd := range tailkitds {
		if tailkitd.Peer != nil && tailkitd.Peer.Online {
			online = append(online, tailkitd)
		}
	}
	return online, nil
}

// PeersFromHosts returns machine peers for a host list.
// It is the usual input shape for tailkit.Nodes.
func PeersFromHosts(hosts []types.Host) []types.Peer {
	peers := make([]types.Peer, 0, len(hosts))
	for _, host := range hosts {
		if host.Peer != nil {
			peers = append(peers, *host.Peer)
		}
	}
	return peers
}

func getPeerCache(ctx context.Context, srv *Server) (peerCacheEntry, error) {
	if entry, ok := srv.getCachedPeers(); ok {
		return entry, nil
	}
	if srv == nil {
		return peerCacheEntry{}, fmt.Errorf("tailkit: server is nil")
	}
	lc := srv.localClient()
	if lc == nil {
		return peerCacheEntry{}, fmt.Errorf("tailkit: local client unavailable")
	}

	status, err := lc.Status(ctx)
	if err != nil {
		return peerCacheEntry{}, err
	}

	entry := peerCacheEntry{
		peers:     status.Peer,
		view:      classifyPeerStatuses(peerStatusSlice(status.Peer)),
		fetchedAt: time.Now(),
	}
	srv.setCachedPeers(entry)
	return entry, nil
}

func (s *Server) peerCacheTTL() time.Duration {
	if s == nil || s.Config.PeerCacheTTL <= 0 {
		return 15 * time.Minute
	}
	return s.Config.PeerCacheTTL
}

func (s *Server) getCachedPeers() (peerCacheEntry, bool) {
	if s == nil {
		return peerCacheEntry{}, false
	}
	s.peerCacheMu.RLock()
	defer s.peerCacheMu.RUnlock()

	if len(s.peerCache.peers) == 0 {
		return peerCacheEntry{}, false
	}
	if time.Since(s.peerCache.fetchedAt) > s.peerCacheTTL() {
		return peerCacheEntry{}, false
	}
	return s.peerCache, true
}

func (s *Server) setCachedPeers(entry peerCacheEntry) {
	if s == nil {
		return
	}
	s.peerCacheMu.Lock()
	defer s.peerCacheMu.Unlock()
	s.peerCache = entry
}

// ─── Classification ─────────────────────────────────────────────────────────────

type peerClassification struct {
	peers     []types.Peer
	hosts     map[key.NodePublic]*types.Host
	tailkitds map[key.NodePublic]*types.Tailkitd
}

func classifyPeerStatuses(statuses []*ipnstate.PeerStatus) peerClassification {
	view := peerClassification{
		peers:     make([]types.Peer, 0, len(statuses)),
		hosts:     make(map[key.NodePublic]*types.Host),
		tailkitds: make(map[key.NodePublic]*types.Tailkitd),
	}

	byHostname := make(map[string]*ipnstate.PeerStatus, len(statuses))
	for _, status := range statuses {
		if status == nil {
			continue
		}
		byHostname[status.HostName] = status
		view.peers = append(view.peers, peerFromStatus(status))
	}

	for _, status := range statuses {
		if status == nil || !isTailkitdSidecar(status.HostName) {
			continue
		}

		hostName := strings.TrimPrefix(status.HostName, "tailkitd-")
		tailkitd := &types.Tailkitd{
			HostName: hostName,
			Peer:     peerPtrFromStatus(status),
			Services: []types.Service{},
		}
		view.tailkitds[status.PublicKey] = tailkitd

		hostStatus := byHostname[hostName]
		if hostStatus == nil {
			continue
		}
		hostPeer := peerPtrFromStatus(hostStatus)
		view.hosts[hostStatus.PublicKey] = &types.Host{
			Name:       hostName,
			Tags:       []string{},
			Metadata:   map[string]string{},
			Peer:       hostPeer,
			Tailkitd:   tailkitd,
			TSHostname: hostPeer.HostName,
			TSDNSName:  hostPeer.DNSName,
			TSIPs:      hostPeer.IPs,
			OS:         hostPeer.OS,
			Online:     hostPeer.Online,
		}
	}

	return view
}

func isTailkitdSidecar(hostname string) bool {
	return strings.HasPrefix(hostname, "tailkitd-")
}

func hostReachable(host types.Host) bool {
	return host.Tailkitd != nil && host.Tailkitd.Peer != nil && host.Tailkitd.Peer.Online
}

func machinePeers(peers []types.Peer) []types.Peer {
	machines := make([]types.Peer, 0, len(peers))
	for _, peer := range peers {
		if isTailkitdSidecar(peer.HostName) {
			continue
		}
		machines = append(machines, peer)
	}
	return machines
}

func hostsSlice(hosts map[key.NodePublic]*types.Host) []types.Host {
	out := make([]types.Host, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, *host)
	}
	return out
}

func peerHostname(peer types.Peer) string {
	if peer.HostName != "" {
		return peer.HostName
	}
	if peer.Status != nil {
		return peer.Status.HostName
	}
	return ""
}

func peerStatusSlice(peers map[key.NodePublic]*ipnstate.PeerStatus) []*ipnstate.PeerStatus {
	statuses := make([]*ipnstate.PeerStatus, 0, len(peers))
	for _, peer := range peers {
		statuses = append(statuses, peer)
	}
	return statuses
}

func peerPtrFromStatus(status *ipnstate.PeerStatus) *types.Peer {
	peer := peerFromStatus(status)
	return &peer
}

func peerFromStatus(status *ipnstate.PeerStatus) types.Peer {
	if status == nil {
		return types.Peer{IPs: []string{}}
	}
	ips := make([]string, 0, len(status.TailscaleIPs))
	for _, ip := range status.TailscaleIPs {
		ips = append(ips, ip.String())
	}
	return types.Peer{
		ID:        string(status.ID),
		PublicKey: status.PublicKey.String(),
		HostName:  status.HostName,
		DNSName:   status.DNSName,
		IPs:       ips,
		OS:        status.OS,
		Online:    status.Online,
		Status:    status,
	}
}
