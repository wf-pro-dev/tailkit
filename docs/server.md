# Server & auth

## NewServer

Constructs and starts a tsnet server. Handles auth key resolution, state directory setup, and graceful shutdown on `SIGTERM`/`SIGINT`.

```go
srv, err := tailkit.NewServer(tailkit.ServerConfig{
    Hostname:  "devbox",
    AuthKey:   os.Getenv("TS_AUTHKEY"),   // falls back to TS_AUTHKEY env var if empty
    StateDir:  "",                         // defaults to $XDG_CONFIG_HOME/tailkit/<hostname>
    Ephemeral: false,
})
defer srv.Close()
```

`ServerConfig` fields:

| Field | Default | Notes |
|---|---|---|
| `Hostname` | — | Required. tsnet hostname. |
| `AuthKey` | `""` | Falls back to `TS_AUTHKEY` env var. |
| `StateDir` | `$XDG_CONFIG_HOME/tailkit/<hostname>` | Where tsnet stores its state. |
| `Ephemeral` | `false` | If true, the node is removed from the tailnet on close. |

---

## Serving HTTP(S)

`Server` exposes helpers for listening on the tsnet interface. Both use the tsnet dialer, so traffic never leaves the tailnet.

```go
// HTTPS — uses Tailscale-issued certificate
err = srv.ListenAndServeTLS(":443", mux)

// HTTP
err = srv.ListenAndServe(":80", mux)

// Raw tls.Config if you manage your own http.Server
cfg := srv.TLSConfig()
```

---

## AuthMiddleware

Runs `lc.WhoIs` on every inbound request. Injects caller identity into the request context. Rejects requests from non-tailnet peers with `401`.

```go
var h http.Handler = yourMux
h = tailkit.AuthMiddleware(srv)(h)
```

Retrieve caller identity inside a handler:

```go
id, ok := tailkit.CallerFromContext(r.Context())
// id.Hostname      — Tailscale ComputedName
// id.TailscaleIP   — first Tailscale address
// id.UserLogin     — Tailscale user login name
// id.Caps          — map[string]bool of granted ACL capabilities
```

Check a specific capability:

```go
if !id.HasCap("tailscale.com/cap/devbox") {
    http.Error(w, "forbidden", http.StatusForbidden)
    return
}
```

`HasCap` is a helper on `CallerIdentity` — equivalent to `id.Caps[capURL]`.
