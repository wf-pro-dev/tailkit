package tailkit

import (
	"testing"
	"time"

	"tailscale.com/tsnet"
)

func TestNewServerInitializesHTTPClients(t *testing.T) {
	srv := newServer(&tsnet.Server{})

	if srv.httpTransport == nil {
		t.Fatal("expected shared transport to be initialized")
	}
	if srv.httpClient == nil {
		t.Fatal("expected normal client to be initialized")
	}
	if srv.streamHTTPClient == nil {
		t.Fatal("expected stream client to be initialized")
	}
	if got := srv.HTTPClient(); got != srv.httpClient {
		t.Fatal("expected HTTPClient to return the normal client")
	}
	if got := srv.StreamHTTPClient(); got != srv.streamHTTPClient {
		t.Fatal("expected StreamHTTPClient to return the stream client")
	}
	if srv.HTTPClient() == srv.StreamHTTPClient() {
		t.Fatal("expected normal and stream clients to be distinct")
	}
	if got := srv.httpClient.Transport; got != srv.httpTransport {
		t.Fatal("expected normal client to use the shared transport")
	}
	if got := srv.streamHTTPClient.Transport; got != srv.httpTransport {
		t.Fatal("expected stream client to use the shared transport")
	}
	if got := srv.HTTPClient().Timeout; got != 60*time.Second {
		t.Fatalf("expected normal client timeout 60s, got %v", got)
	}
	if got := srv.StreamHTTPClient().Timeout; got != 0 {
		t.Fatalf("expected stream client timeout 0, got %v", got)
	}
}

func TestNodeClientHTTPClientReusesServerClients(t *testing.T) {
	srv := newServer(&tsnet.Server{})
	node := &NodeClient{srv: srv}

	if c1, c2 := node.httpClient(), node.httpClient(); c1 != c2 {
		t.Fatal("expected repeated normal-client calls to reuse the same client")
	}
	if c1, c2 := node.streamHTTPClient(), node.streamHTTPClient(); c1 != c2 {
		t.Fatal("expected repeated stream-client calls to reuse the same client")
	}
}

func TestNodeClientsShareServerHTTPClients(t *testing.T) {
	srv := newServer(&tsnet.Server{})
	nodeA := &NodeClient{srv: srv}
	nodeB := &NodeClient{srv: srv}

	if nodeA.httpClient() != nodeB.httpClient() {
		t.Fatal("expected node clients from the same server to share one normal client")
	}
	if nodeA.streamHTTPClient() != nodeB.streamHTTPClient() {
		t.Fatal("expected node clients from the same server to share one stream client")
	}
}

func TestServerCloseWithSharedTransportDoesNotPanic(t *testing.T) {
	srv := newServer(nil)

	if err := srv.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}
