package types

import "tailscale.com/ipn/ipnstate"

// TailscaleNode is returned by Discover — it identifies a tailnet peer that has a

// specific tool installed.
type Peer struct {
	Status  ipnstate.PeerStatus `json:"status"`
	Tailkit *TailkitPeer        `json:"tailkit"`
}

type TailkitPeer struct {
	Status ipnstate.PeerStatus `json:"status"`
	Tools  []Tool              `json:"tools"`
}