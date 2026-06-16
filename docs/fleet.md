# Fleet & peer discovery

## Peer discovery

Peer functions query via the local Tailscale daemon and cache results per `*tailkit.Server` for 15 minutes by default. Override this per server with `tailkit.ServerConfig.PeerCacheTTL`.

tailkit distinguishes **tailnet machines** from **tailkitd-managed hosts**:

- `ListPeers` — ordinary tailnet machines (sidecar peers excluded)
- `ListHosts` — machines paired with a `tailkitd-<hostname>` sidecar
- `ListTailkitds` — the sidecar peers themselves

For fleet fan-out, start from `ListHosts(..., tailkit.ListOnline)` and convert with `PeersFromHosts`.

```go
// online tailnet machines (any role)
peers, err := tailkit.ListPeers(ctx, srv, tailkit.ListOnline)

// all tailnet machines, online or offline
peers, err = tailkit.ListPeers(ctx, srv, tailkit.ListAll)

// tailkitd-managed hosts (online or offline)
hosts, err := tailkit.ListHosts(ctx, srv, tailkit.ListAll)

// tailkitd-managed hosts with an online sidecar (use for fleet ops)
hosts, err = tailkit.ListHosts(ctx, srv, tailkit.ListOnline)
peers = tailkit.PeersFromHosts(hosts)

// tailkitd sidecar peers (hostnames prefixed "tailkitd-")
tailkitds, err := tailkit.ListTailkitds(ctx, srv, tailkit.ListAll)
onlineTailkitds, err := tailkit.ListTailkitds(ctx, srv, tailkit.ListOnline)
```

---

## Nodes (fleet fan-out)

`tailkit.Nodes` fans out requests to a given peer list concurrently with bounded parallelism (10 concurrent). One node failing never aborts the whole fan-out — errors are collected per node.

Pass peers derived from reachable hosts so each peer maps to a reachable tailkitd sidecar.

```go
hosts, err := tailkit.ListHosts(ctx, srv, tailkit.ListOnline)
fleet := tailkit.Nodes(srv, tailkit.PeersFromHosts(hosts))

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
cpuByNode, errs := tailkit.Nodes(srv, tailkit.PeersFromHosts(hosts)).Metrics().CPU(ctx)
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
