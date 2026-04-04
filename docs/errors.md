# Errors

tailkit maps HTTP responses from tailkitd to typed sentinel errors. Use `errors.Is` to check them.

```go
result, err := tailkit.Node(srv, "vps-1").Send(ctx, req)
if errors.Is(err, tailkit.ErrReceiveNotConfigured) {
    // node has no files.toml — skip it
}
```

## Sentinel errors

| Error | Condition |
|---|---|
| `tailkit.ErrReceiveNotConfigured` | Node has no `files.toml`, or the path is not in an allowed write rule |
| `tailkit.ErrToolNotFound` | Tool not installed on the node |
| `tailkit.ErrCommandNotFound` | Command not registered by the tool |
| `tailkit.ErrDockerUnavailable` | Node has no `docker.toml` or daemon is not running |
| `tailkit.ErrSystemdUnavailable` | Node has no `systemd.toml` or D-Bus is unavailable |
| `tailkit.ErrMetricsUnavailable` | Node has no `metrics.toml` |
| `tailkit.ErrVarScopeNotFound` | The `project/env` scope is not in `vars.toml` |
| `tailkit.ErrPermissionDenied` | ACL cap or node config blocked the operation |

These are defined in `github.com/wf-pro-dev/tailkit/types` and re-exported from the root package.

## Availability helpers

Docker, Systemd, and Metrics clients expose an `Available(ctx)` method that returns `(false, nil)` on `503` instead of an error, making it straightforward to check integration availability before calling other methods.

```go
if ok, _ := tailkit.Node(srv, "vps-1").Docker().Available(ctx); !ok {
    // Docker not configured on this node
}
```
