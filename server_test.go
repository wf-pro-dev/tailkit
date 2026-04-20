package tailkit

import (
	"testing"

	"tailscale.com/tsnet"
)

func TestNewServerInitializesSharedHTTPClient(t *testing.T) {
	srv := newServer(&tsnet.Server{})

	if srv.httpTransport == nil {
		t.Fatal("expected shared transport to be initialized")
	}
	if srv.httpClient == nil {
		t.Fatal("expected shared client to be initialized")
	}
	if got := srv.HTTPClient(); got != srv.httpClient {
		t.Fatal("expected HTTPClient to return the shared client")
	}
	if got := srv.httpClient.Transport; got != srv.httpTransport {
		t.Fatal("expected shared client to use the shared transport")
	}
}

func TestNodeClientHTTPClientReusesServerClient(t *testing.T) {
	srv := newServer(&tsnet.Server{})
	node := &NodeClient{srv: srv}

	c1 := node.httpClient()
	c2 := node.httpClient()

	if c1 != c2 {
		t.Fatal("expected repeated calls to reuse the same client")
	}
}

func TestNodeClientsShareServerHTTPClient(t *testing.T) {
	srv := newServer(&tsnet.Server{})
	nodeA := &NodeClient{srv: srv}
	nodeB := &NodeClient{srv: srv}

	if nodeA.httpClient() != nodeB.httpClient() {
		t.Fatal("expected node clients from the same server to share one client")
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
