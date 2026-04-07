# Fleet & peer discovery

## Peer discovery

Before fanning out to multiple nodes, retrieve the peer list. All peer functions query via the local Tailscale daemon and cache results for 15 minutes (configurable via `tailkit.TTL`).

```go
// all online peers running tailkitd
peers, err := tailkit.OnlinePeers(ctx, srv)

// all peers (online and offline)
peers, err = tailkit.AllPeers(ctx, srv)

// only the tailkitd sidecar peers (hostnames prefixed "tailkitd-")
tailkitPeers, err := tailkit.TailkitPeers(ctx, srv)

// single peer by hostname
peer, err := tailkit.GetTailkitPeer(ctx, srv, "vps-1")

// raw peer map from Tailscale status (cached)
peerMap, err := tailkit.GetPeers(ctx, srv)
```

---

## Nodes (fleet fan-out)

`tailkit.Nodes` fans out requests to a given peer list concurrently with bounded parallelism (10 concurrent). One node failing never aborts the whole fan-out — errors are collected per node.

```go
peers, err := tailkit.OnlinePeers(ctx, srv)
fleet := tailkit.Nodes(srv, peers)

// files config across all nodes (only share=true paths returned per node)
configByNode, errs := fleet.Files().Config(ctx)

// metrics across all nodes
cpuByNode, errs := fleet.Metrics().CPU(ctx)
memByNode, errs := fleet.Metrics().Memory(ctx)
allByNode, errs := fleet.Metrics().All(ctx)
// returns map[nodeName]result, map[nodeName]error

// vars across all nodes
varsByNode, errs := fleet.Vars("myapp", "prod").List(ctx)
errs              = fleet.Vars("myapp", "prod").Set(ctx, "LOG_LEVEL", "debug")
// Set returns only map[nodeName]error — nodes where the scope is absent get ErrVarScopeNotFound
```

Fleet operations respect the caller's context as a per-node timeout. A timed-out node appears in the error map; other nodes are unaffected.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cpuByNode, errs := tailkit.Nodes(srv, peers).Metrics().CPU(ctx)
```

---

## Discover

Find all online peers with a specific tool installed.

```go
// any version
nodes, err := tailkit.Discover(ctx, srv, "devbox")

// minimum version
nodes, err = tailkit.DiscoverVersion(ctx, srv, "devbox", "1.2.0")
// returns []types.Peer
```

---

## Broadcast

Push a file to all online nodes concurrently. Each node that has a matching write rule receives the file. Offline nodes and nodes with no matching rule are skipped — their errors are collected, not propagated.

```go
results, errs := tailkit.Broadcast(ctx, srv, tailkit.SendRequest{
    LocalPath: "/home/user/nginx/api.conf",
    DestPath:  "/etc/nginx/conf.d/api.conf",
})
// returns []types.SendResult, map[nodeName]error
```
