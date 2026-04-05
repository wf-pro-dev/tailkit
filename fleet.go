package tailkit

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/wf-pro-dev/tailkit/types"
	integrationsTypes "github.com/wf-pro-dev/tailkit/types/integrations"
	"golang.org/x/sync/errgroup"
)

const fleetConcurrency = 10

// ─── AllNodes ─────────────────────────────────────────────────────────────────

// FleetClient fans out operations to all online tailkitd nodes.
// Obtain via tailkit.AllNodes(srv).
type FleetClient struct {
	srv   *Server
	peers []types.Peer
}

// Nodes returns a FleetClient that fans out requests to the given peers.
// fans out requests to them with bounded parallelism (10 concurrent).
func Nodes(srv *Server, peers []types.Peer) *FleetClient {
	return &FleetClient{srv: srv, peers: peers}
}

// fanOut runs fn on every peer concurrently, bounded to fleetConcurrency.
// Results and errors are collected per-node — one node's failure never
// prevents results from other nodes.
func fanOut[T any](ctx context.Context, peers []types.Peer, fn func(context.Context, types.Peer) (T, error)) (map[string]T, map[string]error) {
	results := make(map[string]T, len(peers))
	errs := make(map[string]error)

	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fleetConcurrency)

	for _, peer := range peers {
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

// --- Fleet Files ───────────────────────────────────────────────────────────────

// FleetFilesClient fans out files requests to all nodes.
type FleetFilesClient struct{ fleet *FleetClient }

func (f *FleetClient) Files() *FleetFilesClient {
	return &FleetFilesClient{fleet: f}
}

func (ff *FleetFilesClient) Config(ctx context.Context) (map[string]integrationsTypes.FilesConfig, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (integrationsTypes.FilesConfig, error) {
		return Node(ff.fleet.srv, p.Status.HostName).Files().Config(ctx)
	})
}

func (ff *FleetFilesClient) List(ctx context.Context, path string) (map[string][]types.DirEntry, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) ([]types.DirEntry, error) {
		return Node(ff.fleet.srv, p.Status.HostName).Files().List(ctx, path)
	})
}

func (ff *FleetFilesClient) Read(ctx context.Context, path string) (map[string]string, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (string, error) {
		return Node(ff.fleet.srv, p.Status.HostName).Files().Read(ctx, path)
	})
}

func (ff *FleetFilesClient) Stat(ctx context.Context, path string) (map[string]types.FileStat, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (types.FileStat, error) {
		return Node(ff.fleet.srv, p.Status.HostName).Files().Stat(ctx, path)
	})
}

func (ff *FleetFilesClient) Download(ctx context.Context, path string, localPath string) (map[string]string, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (string, error) {
		safeLocalPath := fmt.Sprintf("%s/%s-%s", filepath.Dir(localPath), p.Status.HostName, filepath.Base(localPath))
		return safeLocalPath, Node(ff.fleet.srv, p.Status.HostName).Files().Download(ctx, path, safeLocalPath)
	})
}

func (ff *FleetFilesClient) Send(ctx context.Context, req types.SendRequest) (map[string]types.SendResult, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (types.SendResult, error) {
		return Node(ff.fleet.srv, p.Status.HostName).Files().Send(ctx, req)
	})
}

func (ff *FleetFilesClient) SendDir(ctx context.Context, req types.SendDirRequest) (map[string][]types.SendResult, map[string]error) {
	peers := ff.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) ([]types.SendResult, error) {
		return Node(ff.fleet.srv, p.Status.HostName).Files().SendDir(ctx, req)
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

func (fv *FleetVarsClient) Config(ctx context.Context) (map[string]integrationsTypes.VarsConfig, map[string]error) {
	peers := fv.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (integrationsTypes.VarsConfig, error) {
		return Node(fv.fleet.srv, p.Status.HostName).Vars(fv.project, fv.env).Config(ctx)
	})
}

// Set writes a var to the given scope on every node that has it configured.
// Errors are collected per-node and returned together — one node failing
// does not prevent writes to other nodes.
func (fv *FleetVarsClient) Set(ctx context.Context, key, value string) map[string]error {
	peers := fv.fleet.peers
	_, errs := fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (struct{}, error) {
		err := Node(fv.fleet.srv, p.Status.HostName).Vars(fv.project, fv.env).Set(ctx, key, value)
		return struct{}{}, err
	})
	return errs
}

// List reads the scope from every node. Nodes where the scope is not
// configured return ErrVarScopeNotFound in the error map.
func (fv *FleetVarsClient) List(ctx context.Context) (map[string]map[string]string, map[string]error) {
	peers := fv.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (map[string]string, error) {
		return Node(fv.fleet.srv, p.Status.HostName).Vars(fv.project, fv.env).List(ctx)
	})
}

// ─── Fleet Metrics ────────────────────────────────────────────────────────────

// FleetMetricsClient fans out metrics requests to all nodes.
type FleetMetricsClient struct{ fleet *FleetClient }

func (f *FleetClient) Metrics() *FleetMetricsClient {
	return &FleetMetricsClient{fleet: f}
}

func (fm *FleetMetricsClient) Config(ctx context.Context) (map[string]integrationsTypes.MetricsConfig, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (integrationsTypes.MetricsConfig, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().Config(ctx)
	})
}

func (fm *FleetMetricsClient) CPU(ctx context.Context) (map[string]map[string]any, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (map[string]any, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().CPU(ctx)
	})
}

func (fm *FleetMetricsClient) Memory(ctx context.Context) (map[string]map[string]any, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (map[string]any, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().Memory(ctx)
	})
}

func (fm *FleetMetricsClient) All(ctx context.Context) (map[string]map[string]any, map[string]error) {
	peers := fm.fleet.peers
	return fanOut(ctx, peers, func(ctx context.Context, p types.Peer) (map[string]any, error) {
		return Node(fm.fleet.srv, p.Status.HostName).Metrics().All(ctx)
	})
}
