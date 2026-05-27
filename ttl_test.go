package tailkit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServerPeerCacheTTLFallsBackToDefault(t *testing.T) {
	srv := &Server{}
	if got := srv.peerCacheTTL(); got != 15*time.Minute {
		t.Fatalf("expected default TTL 15m, got %v", got)
	}

	srv.Config.PeerCacheTTL = 5 * time.Second
	if got := srv.peerCacheTTL(); got != 5*time.Second {
		t.Fatalf("expected configured TTL 5s, got %v", got)
	}
}
