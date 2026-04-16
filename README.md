# tailkit

Go library for building Tailscale-native tools. tailkit has two distinct concerns:

1. **tsnet utilities** — useful for any tailnet tool regardless of tailkitd
2. **tailkitd client SDK** — typed HTTP client for every tailkitd endpoint

Tools built with tailkit get consistent auth, peer discovery, and access to node-level integrations (Docker, systemd, metrics, files, vars, exec) across every node running [`tailkitd`](https://github.com/wf-pro-dev/tailkitd).

Recent additions include first-class SSE stream support for exec jobs, Docker logs/stats, systemd journal tails, and metrics streams including TCP listen-port discovery.

---

## Install

```
go get github.com/wf-pro-dev/tailkit
```

---

## Quick start

```go
srv, err := tailkit.NewServer(tailkit.ServerConfig{
    Hostname: "devbox",
    AuthKey:  os.Getenv("TS_AUTHKEY"),
})
defer srv.Close()

// register this tool with tailkitd on startup
tailkit.Install(ctx, types.Tool{Name: "devbox", Version: build.Version, TsnetHost: "devbox"})

// single node
containers, err := tailkit.Node(srv, "vps-1").Docker().Containers(ctx)
err = tailkit.Node(srv, "vps-1").Metrics().StreamPorts(ctx, func(e tailkit.PortEvent) error {
    switch e.Kind {
    case "snapshot":
        // replace current local state with e.Ports
    case "bound", "released":
        // update local state with e.Port
    }
    return nil
})

// fleet
peers, err := tailkit.OnlinePeers(ctx, srv)
cpuByNode, errs := tailkit.Nodes(srv, peers).Metrics().CPU(ctx)
```

---

## Streaming APIs

tailkit now exposes both a generic SSE client and typed stream helpers:

- `node.Stream(ctx, path, fn)` for raw SSE access
- `node.ExecJobStream(...)`
- `node.Docker().StreamLogs(...)`
- `node.Docker().StreamStats(...)`
- `node.Systemd().StreamJournal(...)`
- `node.Systemd().StreamSystemJournal(...)`
- `node.Metrics().StreamCPU(...)`
- `node.Metrics().StreamMemory(...)`
- `node.Metrics().StreamNetwork(...)`
- `node.Metrics().StreamProcesses(...)`
- `node.Metrics().StreamAll(...)`
- `node.Metrics().Ports()`
- `node.Metrics().PortsAvailable()`
- `node.Metrics().StreamPorts(...)`

The new metrics port APIs expose TCP listeners as `tailkit.ListenPort` values and stream changes as `tailkit.PortEvent`.

---

## Docs

| Document | Description |
|---|---|
| [server.md](docs/server.md) | `NewServer`, `ServerConfig`, TLS helpers, `AuthMiddleware` |
| [node.md](docs/node.md) | `Node`, `Tools`, `Files`, `Vars`, `Docker`, `Systemd`, `Metrics` |
| [fleet.md](docs/fleet.md) | `Nodes`, `Discover`, `Broadcast`, peer discovery primitives |
| [errors.md](docs/errors.md) | Typed errors and how to check them |

## License

MIT
