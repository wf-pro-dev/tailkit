package tailkit

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

const fleetConcurrency = 10

// ─── AllNodes ─────────────────────────────────────────────────────────────────

// FleetClient fans out operations to all online tailkitd nodes.
// Obtain via tailkit.AllNodes(srv).
type FleetClient struct {
	srv   *Server
	peers []Peer
}

// Nodes returns a FleetClient that fans out requests to the given peers.
// fans out requests to them with bounded parallelism (10 concurrent).
func Nodes(srv *Server, peers []Peer) *FleetClient {
	return &FleetClient{srv: srv, peers: peers}
}

// fanOut runs fn on every peer concurrently, bounded to fleetConcurrency.
// Results and errors are collected per-node — one node's failure never
// prevents results from other nodes.
func fanOut[T any](ctx context.Context, peers []Peer, fn func(context.Context, Peer) (T, error)) (map[string]T, map[string]error) {
	results := make(map[string]T, len(peers))
	errs := make(map[string]error)

	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fleetConcurrency)

	for _, peer := range peers {
		peer := peer
		g.Go(func() error {
			val, err := fn(ctx, peer)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[peer.Status.HostName] = err
			} else {
				results[peer.Status.HostName] = val
			}
			return nil // never propagate — collect per-node
		})
	}
	_ = g.Wait()
	return results, errs
}

// ─── Fleet Metrics ────────────────────────────────────────────────────────────

// FleetMetricsClient fans out metrics requests to all nodes.
type FleetMetricsClient struct{ fleet *FleetClient }

func (f *FleetClient) Metrics() *FleetMetricsClient {
	return &FleetMetricsClient{fleet: f}
}

func (fm *FleetMetricsClient) CPU(ctx context.Context) (map[string]map[string]any, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p Peer) (map[string]any, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().CPU(ctx)
	})
}

func (fm *FleetMetricsClient) Memory(ctx context.Context) (map[string]map[string]any, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p Peer) (map[string]any, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().Memory(ctx)
	})
}

func (fm *FleetMetricsClient) All(ctx context.Context) (map[string]map[string]any, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p Peer) (map[string]any, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().All(ctx)
	})
}

// ─── Fleet Vars ───────────────────────────────────────────────────────────────

// FleetVarsClient fans out var operations across all nodes.
type FleetVarsClient struct {
	fleet   *FleetClient
	project string
	env     string
}

func (f *FleetClient) Vars(project, env string) *FleetVarsClient {
	return &FleetVarsClient{fleet: f, project: project, env: env}
}

// Set writes a var to the given scope on every node that has it configured.
// Errors are collected per-node and returned together — one node failing
// does not prevent writes to other nodes.
func (fv *FleetVarsClient) Set(ctx context.Context, key, value string) map[string]error {
	peers := fv.fleet.peers
	_, errs := fanOut(ctx, peers, func(ctx context.Context, p Peer) (struct{}, error) {
		err := Node(fv.fleet.srv, p.Status.HostName).Vars(fv.project, fv.env).Set(ctx, key, value)
		return struct{}{}, err
	})
	return errs
}

// List reads the scope from every node. Nodes where the scope is not
// configured return ErrVarScopeNotFound in the error map.
func (fv *FleetVarsClient) List(ctx context.Context) (map[string]map[string]string, map[string]error) {
	peers := fv.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p Peer) (map[string]string, error) {
		return Node(fv.fleet.srv, p.Status.HostName).Vars(fv.project, fv.env).List(ctx)
	})
}

// ─── Broadcast ────────────────────────────────────────────────────────────────

// Broadcast pushes a file to all online nodes concurrently.
// Each node that has a matching write rule receives the file.
// Nodes that are offline or have no matching write rule are skipped —
// their errors are collected and returned, not propagated.
func Broadcast(ctx context.Context, srv *Server, req SendRequest) ([]SendResult, map[string]error) {
	peers, err := OnlinePeers(ctx, srv)
	if err != nil {
		return nil, map[string]error{"_discover": err}
	}

	results := make([]SendResult, 0, len(peers))
	errs := make(map[string]error)
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fleetConcurrency)

	for _, peer := range peers {
		peer := peer
		g.Go(func() error {
			result, err := Node(srv, peer.Status.HostName).Send(ctx, req)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[peer.Status.HostName] = err
			} else {
				results = append(results, result)
			}
			return nil
		})
	}
	_ = g.Wait()
	return results, errs
}
