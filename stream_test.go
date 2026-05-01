package tailkit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wf-pro-dev/tailkit/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestStreamUsesStreamHTTPClient(t *testing.T) {
	timedCalls := 0
	streamCalls := 0

	node := &NodeClient{
		srv: &Server{
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					timedCalls++
					return nil, context.DeadlineExceeded
				}),
			},
			streamHTTPClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					streamCalls++
					if got := req.Header.Get("Accept"); got != "text/event-stream" {
						t.Fatalf("expected SSE accept header, got %q", got)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("event: metrics.cpu\ndata: {\"info\":[],\"percent_per_cpu\":[],\"percent_total\":0}\n\n")),
						Header:     make(http.Header),
					}, nil
				}),
			},
		},
		tailkitd: &types.TailkitPeer{},
	}
	node.tailkitd.Status.HostName = "example.test"

	calls := 0
	err := Stream(context.Background(), node, "/metrics/cpu/stream", []string{EventCPU}, func(event types.Event[types.CPU]) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one event callback, got %d", calls)
	}
	if timedCalls != 0 {
		t.Fatalf("expected timed client to be unused, got %d calls", timedCalls)
	}
	if streamCalls != 1 {
		t.Fatalf("expected stream client to be used once, got %d calls", streamCalls)
	}
}

func TestNodeDoUsesTimedHTTPClient(t *testing.T) {
	timedCalls := 0
	streamCalls := 0

	node := &NodeClient{
		srv: &Server{
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					timedCalls++
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"available":true}`)),
						Header:     make(http.Header),
					}, nil
				}),
			},
			streamHTTPClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					streamCalls++
					return nil, context.DeadlineExceeded
				}),
			},
		},
		tailkitd: &types.TailkitPeer{},
	}
	node.tailkitd.Status.HostName = "example.test"

	var out map[string]bool
	if err := node.do(context.Background(), http.MethodGet, "/metrics/ports/available", nil, &out); err != nil {
		t.Fatalf("do returned error: %v", err)
	}
	if !out["available"] {
		t.Fatal("expected decoded response from timed client")
	}
	if timedCalls != 1 {
		t.Fatalf("expected timed client to be used once, got %d calls", timedCalls)
	}
	if streamCalls != 0 {
		t.Fatalf("expected stream client to be unused, got %d calls", streamCalls)
	}
}
