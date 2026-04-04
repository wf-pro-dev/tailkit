# tailkit

Go library for building Tailscale-native tools. tailkit has two distinct concerns:

1. **tsnet utilities** — useful for any tailnet tool regardless of tailkitd
2. **tailkitd client SDK** — typed HTTP client for every tailkitd endpoint

Tools built with tailkit get consistent auth, peer discovery, and access to node-level integrations (Docker, systemd, metrics, files, vars, exec) across every node running [`tailkitd`](https://github.com/wf-pro-dev/tailkitd).

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

// fleet
peers, err := tailkit.OnlinePeers(ctx, srv)
cpuByNode, errs := tailkit.Nodes(srv, peers).Metrics().CPU(ctx)
```

---

## Docs

| Document | Description |
|---|---|
| [server.md](docs/server.md) | `NewServer`, `ServerConfig`, TLS helpers, `AuthMiddleware` |
| [node.md](docs/node.md) | `Node`, `Tools`, `Files`, `Vars`, `Docker`, `Systemd`, `Metrics` |
| [fleet.md](docs/fleet.md) | `Nodes`, `Discover`, `Broadcast`, peer discovery primitives |
| [errors.md](docs/errors.md) | Typed errors and how to check them |

```

## License

MIT
