
package tailkit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminClientSendsHeaderAndMapsUnauthorized(t *testing.T) {
	node := &NodeClient{
		srv: &Server{
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if got := req.Header.Get("X-Admin-Key"); got != "bad-key" {
						t.Fatalf("expected X-Admin-Key header, got %q", got)
					}
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader(`{"error":"nope"}`)),
						Header:     make(http.Header),
					}, nil
				}),
			},
		},
		hostname: "node-01",
	}

	err := node.Admin("bad-key").PushHostConfig(context.Background(), HostConfig{Name: "node-01"})
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error")
	}
	if !strings.Contains(err.Error(), "push host config") {
		t.Fatalf("expected wrapped operation context, got %v", err)
	}
	if mapped := err.Error(); !strings.Contains(mapped, "unauthorized") {
		t.Fatalf("expected unauthorized in error, got %v", err)
	}
}
