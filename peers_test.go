package tailkit

import (
	"net/netip"
	"testing"
	"time"

	"github.com/wf-pro-dev/tailkit/client"
	"go4.org/mem"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

func TestPeerListingFiltersSidecars(t *testing.T) {
	normal := peerStatusForTest("node-01", true, 1)
	sidecar := peerStatusForTest("tailkitd-node-01", true, 2)
	offline := peerStatusForTest("node-02", false, 3)

	view := classifyPeerStatuses([]*ipnstate.PeerStatus{normal, sidecar, offline})
	machines := machinePeers(view.peers)

	if got, want := len(machines), 2; got != want {
		t.Fatalf("expected %d machine peers, got %d", want, got)
	}
	for _, peer := range machines {
		if isTailkitdSidecar(peer.HostName) {
			t.Fatalf("sidecar leaked into machine peers: %q", peer.HostName)
		}
	}
}

func TestOnlineHostsRequiresReachableSidecar(t *testing.T) {
	onlineHost := peerStatusForTest("node-01", true, 1)
	onlineSidecar := peerStatusForTest("tailkitd-node-01", true, 2)
	offlineHost := peerStatusForTest("node-02", false, 3)
	offlineSidecar := peerStatusForTest("tailkitd-node-02", false, 4)

	view := classifyPeerStatuses([]*ipnstate.PeerStatus{
		onlineHost, onlineSidecar, offlineHost, offlineSidecar,
	})

	hosts := hostsSlice(view.hosts)
	if got, want := len(hosts), 2; got != want {
		t.Fatalf("expected %d hosts, got %d", want, got)
	}

	reachable := 0
	for _, host := range hosts {
		if hostReachable(host) {
			reachable++
		}
	}
	if reachable != 1 {
		t.Fatalf("expected 1 reachable host, got %d", reachable)
	}
}

func TestHostPeersReturnsMachinePeers(t *testing.T) {
	host := client.Host{
		Name: "node-01",
		Peer: &client.Peer{HostName: "node-01", Online: true},
	}
	peers := PeersFromHosts([]client.Host{host, {Name: "missing-peer"}})
	if len(peers) != 1 || peers[0].HostName != "node-01" {
		t.Fatalf("unexpected host peers: %#v", peers)
	}
}

func TestClassifyPeerStatuses(t *testing.T) {
	normal := peerStatusForTest("node-01", true, 1)
	host := peerStatusForTest("node-02", true, 2)
	sidecar := peerStatusForTest("tailkitd-node-02", true, 3)
	orphan := peerStatusForTest("tailkitd-missing", true, 4)
	offline := peerStatusForTest("node-03", false, 5)

	view := classifyPeerStatuses([]*ipnstate.PeerStatus{normal, host, sidecar, orphan, offline})

	if got, want := len(view.peers), 5; got != want {
		t.Fatalf("expected %d peers, got %d", want, got)
	}
	if got, want := len(view.hosts), 1; got != want {
		t.Fatalf("expected %d host, got %d", want, got)
	}
	if got, want := len(view.tailkitds), 2; got != want {
		t.Fatalf("expected %d tailkitds, got %d", want, got)
	}

	classified := view.hosts[host.PublicKey]
	if classified == nil {
		t.Fatal("expected node-02 to be classified as a host")
	}
	if classified.Peer.HostName != "node-02" {
		t.Fatalf("unexpected host peer: %#v", classified.Peer)
	}
	if classified.Tailkitd == nil || classified.Tailkitd.Peer.HostName != "tailkitd-node-02" {
		t.Fatalf("unexpected tailkitd link: %#v", classified.Tailkitd)
	}
	if view.hosts[normal.PublicKey] != nil {
		t.Fatal("normal peer without sidecar should not be classified as host")
	}
	if view.hosts[offline.PublicKey] != nil {
		t.Fatal("offline peer without sidecar should not be classified as host")
	}
}

func TestPeerCacheIsServerScoped(t *testing.T) {
	first := &Server{}
	second := &Server{}
	firstStatus := peerStatusForTest("node-01", true, 1)
	secondStatus := peerStatusForTest("node-02", true, 2)

	first.setCachedPeers(peerCacheEntry{
		peers: map[key.NodePublic]*ipnstate.PeerStatus{
			firstStatus.PublicKey: firstStatus,
		},
		view:      classifyPeerStatuses([]*ipnstate.PeerStatus{firstStatus}),
		fetchedAt: timeNowForTest(),
	})
	second.setCachedPeers(peerCacheEntry{
		peers: map[key.NodePublic]*ipnstate.PeerStatus{
			secondStatus.PublicKey: secondStatus,
		},
		view:      classifyPeerStatuses([]*ipnstate.PeerStatus{secondStatus}),
		fetchedAt: timeNowForTest(),
	})

	firstPeers, ok := first.getCachedPeers()
	if !ok {
		t.Fatal("expected first server cache")
	}
	secondPeers, ok := second.getCachedPeers()
	if !ok {
		t.Fatal("expected second server cache")
	}
	if firstPeers.peers[firstStatus.PublicKey] == nil || firstPeers.peers[secondStatus.PublicKey] != nil {
		t.Fatal("first server cache leaked or lost peer data")
	}
	if secondPeers.peers[secondStatus.PublicKey] == nil || secondPeers.peers[firstStatus.PublicKey] != nil {
		t.Fatal("second server cache leaked or lost peer data")
	}
}

func peerStatusForTest(hostname string, online bool, seed byte) *ipnstate.PeerStatus {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed
	}
	return &ipnstate.PeerStatus{
		ID:           tailcfg.StableNodeID(hostname),
		PublicKey:    key.NodePublicFromRaw32(mem.B(raw)),
		HostName:     hostname,
		DNSName:      hostname + ".tailnet.ts.net.",
		OS:           "linux",
		TailscaleIPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")},
		Online:       online,
	}
}

func timeNowForTest() time.Time {
	return time.Now()
}
