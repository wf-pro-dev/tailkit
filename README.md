[tailkit-README.md](https://github.com/user-attachments/files/26091942/tailkit-README.md)
# tailkit

Go library for building Tailscale-native tools. tailkit has two distinct concerns:

1. **tsnet utilities** — useful for any tailnet tool regardless of tailkitd
2. **tailkitd client SDK** — typed HTTP client for every tailkitd endpoint

Tools built with tailkit get consistent auth, peer discovery, and access to node-level integrations (Docker, systemd, metrics, files, vars, exec) across every node running [`tailkitd`](https://github.com/wf-pro-dev/tailkitd).

---

## Install

```bash
go get github.com/wf-pro-dev/tailkit
```

---

## tsnet utilities

These are useful for any tsnet-based tool, regardless of whether tailkitd is installed on the target nodes.

### NewServer

Handles the three things every tool gets wrong independently: auth key resolution, TLS setup via `lc.GetCertificate`, and graceful shutdown on OS signal.

```go
srv, err := tailkit.NewServer(tailkit.ServerConfig{
    Hostname: "devbox",
    AuthKey:  os.Getenv("TS_AUTHKEY"),
})
defer srv.Close()
```

### AuthMiddleware

Runs `lc.WhoIs` on every inbound request. Injects caller hostname, Tailscale IP, login name, and application capabilities into the request context. Replaces the `callerHost` + manual `WhoIs` boilerplate every tool currently reimplements.

```go
var h http.Handler = yourMux
h = tailkit.AuthMiddleware(srv)(h)
```

Retrieve caller identity inside a handler:

```go
id, ok := tailkit.CallerFromContext(r.Context())
// id.Hostname, id.TailscaleIP, id.UserLogin, id.Caps
```

---

## Tool registration

Tools call `tailkit.Install()` once at install time and on every upgrade. This writes a JSON file to `/etc/tailkitd/tools/{name}.json` that tailkitd reads to populate its tool registry and exec command list.

```go
err := tailkit.Install(ctx, tailkit.Tool{
    Name:      "devbox",
    Version:   build.Version,
    TsnetHost: "devbox",            // the tsnet hostname this tool registers
    Commands: []tailkit.Command{
        {
            Name:        "reload-nginx",
            Description: "Reloads nginx after a config push",
            ACLCap:      "tailscale.com/cap/devbox",
            ExecParts:   []string{"/usr/bin/systemctl", "reload", "nginx"},
            Timeout:     30 * time.Second,
        },
        {
            Name:        "restart-container",
            Description: "Restarts a named Docker container",
            ACLCap:      "tailscale.com/cap/devbox",
            ExecParts:   []string{"/usr/bin/docker", "restart", "{{.container}}"},
            Timeout:     60 * time.Second,
            Args: []tailkit.Arg{
                {
                    Name:     "container",
                    Type:     "string",
                    Required: true,
                    Pattern:  tailkit.PatternIdentifier,
                },
            },
        },
    },
})
```

Uninstall removes the registration file:

```go
err := tailkit.Uninstall("devbox")
```

### Arg pattern constants

```go
tailkit.PatternIdentifier  // ^[a-zA-Z0-9_-]+$       container names, service names
tailkit.PatternPath        // ^(/[a-zA-Z0-9_./-]+)+$  absolute unix paths
tailkit.PatternSemver      // ^v?[0-9]+\.[0-9]+\.[0-9]+$
tailkit.PatternIP          // ^(\d{1,3}\.){3}\d{1,3}$
tailkit.PatternPort        // ^([1-9][0-9]{0,4})$
tailkit.PatternFilename    // ^[a-zA-Z0-9_.,-]+$
```

---

## Node client

All client methods are accessed via `tailkit.Node(srv, hostname)`. The `tsnet.Server` is the caller's own server — tailkit uses it to open a direct tsnet connection to `tailkitd.<hostname>.ts.net`.

### Tools

```go
tools, err := tailkit.Node(srv, "vps-1").Tools(ctx)
ok, err    := tailkit.Node(srv, "vps-1").HasTool(ctx, "devbox", "1.2.0")
```

### Exec

```go
// fire and forget — returns immediately with job_id
job, err := tailkit.Node(srv, "vps-1").Exec(ctx, "devbox", "reload-nginx", nil)

// fire and poll until complete — blocks until done or context cancelled
result, err := tailkit.Node(srv, "vps-1").ExecWait(ctx, "devbox", "restart-container",
    map[string]string{"container": "my-app"},
)

// poll an existing job
result, err := tailkit.Node(srv, "vps-1").ExecJob(ctx, job.JobID)
```

### Files

```go
// read file content as string (Accept: application/json)
content, err := tailkit.Node(srv, "vps-1").Files().Read(ctx, "/opt/myapp/compose.yml")

// download to a local path (Accept: application/octet-stream)
err = tailkit.Node(srv, "vps-1").Files().Download(ctx,
    "/opt/myapp/compose.yml",
    "/local/dest/compose.yml",
)

// list directory
entries, err := tailkit.Node(srv, "vps-1").Files().List(ctx, "/opt/myapp/")

// send a local file to the node
result, err := tailkit.Node(srv, "vps-1").Send(ctx, tailkit.SendRequest{
    LocalPath: "/home/user/nginx/api.conf",
    DestPath:  "/etc/nginx/conf.d/api.conf",
})

// send a local directory to the node
result, err := tailkit.Node(srv, "vps-1").SendDir(ctx, tailkit.SendDirRequest{
    LocalDir: "/home/user/nginx/",
    DestPath: "/etc/nginx/conf.d/",
})

// wait for the post-receive hook to complete
if result.JobID != "" {
    hookResult, err := tailkit.Node(srv, "vps-1").ExecJob(ctx, result.JobID)
}
```

### Vars

```go
// list all vars in a scope
vars, err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").List(ctx)

// get single var
val, err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").Get(ctx, "DATABASE_URL")

// set a var
err = tailkit.Node(srv, "vps-1").Vars("myapp", "prod").Set(ctx, "LOG_LEVEL", "debug")

// delete a var
err = tailkit.Node(srv, "vps-1").Vars("myapp", "prod").Delete(ctx, "LOG_LEVEL")

// render as KEY=VALUE text
envText, err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").Env(ctx)
```

### ExecWith

Fetch vars from a node and inject them into a local process without touching disk. Secrets exist only in the process environment and disappear when the process exits.

```go
vars, err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").List(ctx)
err        = tailkit.ExecWith(ctx, vars, []string{"/usr/bin/node", "server.js"})
```

### Docker

```go
// containers
containers, err := tailkit.Node(srv, "vps-1").Docker().Containers(ctx)
detail, err     := tailkit.Node(srv, "vps-1").Docker().Container(ctx, "my-app")
logs, err       := tailkit.Node(srv, "vps-1").Docker().Logs(ctx, "my-app", 100)
stats, err      := tailkit.Node(srv, "vps-1").Docker().Stats(ctx, "my-app")
err              = tailkit.Node(srv, "vps-1").Docker().Start(ctx, "my-app")
err              = tailkit.Node(srv, "vps-1").Docker().Stop(ctx, "my-app")
err              = tailkit.Node(srv, "vps-1").Docker().Restart(ctx, "my-app")
err              = tailkit.Node(srv, "vps-1").Docker().Remove(ctx, "my-app")

// images
images, err := tailkit.Node(srv, "vps-1").Docker().Images(ctx)
job, err    := tailkit.Node(srv, "vps-1").Docker().Pull(ctx, "nginx:latest")

// compose
projects, err := tailkit.Node(srv, "vps-1").Docker().Compose().Projects(ctx)
project, err  := tailkit.Node(srv, "vps-1").Docker().Compose().Project(ctx, "myapp")
job, err      := tailkit.Node(srv, "vps-1").Docker().Compose().Up(ctx, "myapp", "/opt/myapp/compose.yml")
job, err       = tailkit.Node(srv, "vps-1").Docker().Compose().Down(ctx, "myapp")
job, err       = tailkit.Node(srv, "vps-1").Docker().Compose().Pull(ctx, "myapp")
job, err       = tailkit.Node(srv, "vps-1").Docker().Compose().Restart(ctx, "myapp")
job, err       = tailkit.Node(srv, "vps-1").Docker().Compose().Build(ctx, "myapp")

// swarm (read only in v1)
nodes, err    := tailkit.Node(srv, "vps-1").Docker().Swarm().Nodes(ctx)
services, err := tailkit.Node(srv, "vps-1").Docker().Swarm().Services(ctx)
tasks, err    := tailkit.Node(srv, "vps-1").Docker().Swarm().Tasks(ctx)

// availability check — returns false on 503, never errors
available, err := tailkit.Node(srv, "vps-1").Docker().Available(ctx)
```

### Systemd

```go
units, err   := tailkit.Node(srv, "vps-1").Systemd().Units(ctx)
unit, err    := tailkit.Node(srv, "vps-1").Systemd().Unit(ctx, "nginx.service")
file, err    := tailkit.Node(srv, "vps-1").Systemd().UnitFile(ctx, "nginx.service")
job, err     := tailkit.Node(srv, "vps-1").Systemd().Start(ctx, "nginx.service")
job, err      = tailkit.Node(srv, "vps-1").Systemd().Stop(ctx, "nginx.service")
job, err      = tailkit.Node(srv, "vps-1").Systemd().Restart(ctx, "nginx.service")
job, err      = tailkit.Node(srv, "vps-1").Systemd().Reload(ctx, "nginx.service")
job, err      = tailkit.Node(srv, "vps-1").Systemd().Enable(ctx, "nginx.service")
job, err      = tailkit.Node(srv, "vps-1").Systemd().Disable(ctx, "nginx.service")
entries, err := tailkit.Node(srv, "vps-1").Systemd().Journal(ctx, "nginx.service", 100)
entries, err  = tailkit.Node(srv, "vps-1").Systemd().SystemJournal(ctx, 100)
available, _  := tailkit.Node(srv, "vps-1").Systemd().Available(ctx)
```

### Metrics

```go
host, err      := tailkit.Node(srv, "vps-1").Metrics().Host(ctx)
cpu, err       := tailkit.Node(srv, "vps-1").Metrics().CPU(ctx)
memory, err    := tailkit.Node(srv, "vps-1").Metrics().Memory(ctx)
disks, err     := tailkit.Node(srv, "vps-1").Metrics().Disk(ctx)
network, err   := tailkit.Node(srv, "vps-1").Metrics().Network(ctx)
processes, err := tailkit.Node(srv, "vps-1").Metrics().Processes(ctx)
all, err       := tailkit.Node(srv, "vps-1").Metrics().All(ctx)
available, _   := tailkit.Node(srv, "vps-1").Metrics().Available(ctx)
```

---

## Fleet client

`tailkit.AllNodes` discovers all online peers via `lc.Status()`, fans out requests concurrently with bounded parallelism, and returns results and errors per node. One node failing never aborts the whole fan-out.

```go
// same metric across every online node
cpuByNode, errs := tailkit.AllNodes(srv).Metrics().CPU(ctx)
// returns map[nodeName]cpu.Result, map[nodeName]error

// discover nodes with a specific tool installed
nodes, err := tailkit.Discover(ctx, srv, "devbox")
// returns []NodeInfo — node name, tailscale IP, and the matching Tool entry

// broadcast a file to all nodes with a matching receiver configured
results, err := tailkit.Broadcast(ctx, srv, tailkit.SendRequest{
    LocalPath: "/home/user/nginx/api.conf",
    DestPath:  "/etc/nginx/conf.d/api.conf",
})
// returns []SendResult — one per node, errors collected not propagated

// push a var to all nodes that have the scope in their vars.toml
err = tailkit.AllNodes(srv).Vars("myapp", "prod").Set(ctx, "LOG_LEVEL", "debug")
```

---

## Logging

tailkit itself does not log. It is a library — logging decisions belong to the tool that imports it.

When debugging a tool built on tailkit, enable tailkitd's development logs on the target node to see the full request lifecycle including permission checks, exec invocations, and integration responses:

```bash
# on the target node — restart tailkitd with development logging
TAILKITD_ENV=development systemctl restart tailkitd

# follow logs
journalctl -u tailkitd -f
```

tailkitd logs carry a `component` field per integration and a `caller` field showing which node made the request, making it straightforward to isolate traffic from a specific tool during development.

---

## Context conventions

Every tailkit function that performs I/O accepts `context.Context` as its first parameter. This is standard Go convention and is consistently applied throughout the library — no exceptions.

```go
// all client methods follow this signature
tools, err := tailkit.Node(srv, "vps-1").Tools(ctx)
val,   err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").Get(ctx, "KEY")
err         = tailkit.Install(ctx, tool)
```

Pass a context with a deadline for any operation that should not block indefinitely:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
containers, err := tailkit.Node(srv, "vps-1").Docker().Containers(ctx)
```

Fleet operations respect per-node timeouts. If one node times out, its error is recorded in the error map and the fan-out continues to the remaining nodes — the timeout of one node never blocks results from others:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
cpuByNode, errs := tailkit.AllNodes(srv).Metrics().CPU(ctx)
// nodes that responded within 5s are in cpuByNode
// nodes that timed out are in errs — other nodes are unaffected
```

Async job polling via `ExecWait` uses the caller's context as the polling deadline. Cancelling the context stops polling but does not cancel the running job on the remote node — the job runs to completion (or its declared timeout) regardless.

---

## Error types

```go
tailkit.ErrReceiveNotConfigured  // node has no files.toml
tailkit.ErrToolNotFound          // tool not installed on node
tailkit.ErrCommandNotFound       // command not registered by tool
tailkit.ErrDockerUnavailable     // node has no docker.toml or daemon not running
tailkit.ErrSystemdUnavailable    // node has no systemd.toml or D-Bus unavailable
tailkit.ErrMetricsUnavailable    // node has no metrics.toml
tailkit.ErrVarScopeNotFound      // project/env scope not in vars.toml
tailkit.ErrPermissionDenied      // ACL cap or node config blocked the operation
```

Check errors:

```go
result, err := tailkit.Node(srv, "vps-1").Send(ctx, req)
if errors.Is(err, tailkit.ErrReceiveNotConfigured) {
    // skip this node — not configured for file receive
}
```

---

## Shared types

tailkit imports the following type packages — the same ones tailkitd uses to encode responses. No translation layer between server and client.

```
github.com/docker/docker/api/types/container
github.com/docker/docker/api/types/image
github.com/docker/docker/api/types/swarm
github.com/docker/docker/api/types/network
github.com/docker/docker/api/types/volume
github.com/coreos/go-systemd/v22/dbus
github.com/shirou/gopsutil/v4/cpu
github.com/shirou/gopsutil/v4/mem
github.com/shirou/gopsutil/v4/disk
github.com/shirou/gopsutil/v4/net
github.com/shirou/gopsutil/v4/host
```

---

## Module path

```
github.com/wf-pro-dev/tailkit
```
