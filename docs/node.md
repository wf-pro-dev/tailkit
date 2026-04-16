# Node client

All client methods are accessed via `tailkit.Node(srv, hostname)`. The `*Server` is the caller's own server — tailkit uses it to open a direct tsnet connection to `tailkitd-<hostname>.<tailnet>.ts.net`.

`Node(...)` construction is free — no network calls are made until a method is invoked.

```go
node := tailkit.Node(srv, "vps-1")
```

---

## Streaming

Streaming endpoints use server-sent events (SSE). tailkit exposes them in two layers:

1. `node.Stream(ctx, path, fn)` for raw SSE access
2. Typed helpers such as `ExecJobStream`, `StreamLogs`, and `Metrics().StreamPorts(...)`

Each event arrives as:

```go
type Event struct {
    Name string
    ID   int64
    Data json.RawMessage
}
```

`NodeClient.Stream` automatically sets `Accept: text/event-stream` and re-sends `Last-Event-ID` when reconnecting with a previously seen event ID.

```go
err := node.Stream(ctx, "/integrations/metrics/ports/stream", func(e tailkit.Event) error {
    switch e.Name {
    case tailkit.EventPortsSnapshot:
        var snapshot tailkit.PortEvent
        return json.Unmarshal(e.Data, &snapshot)
    case tailkit.EventPortBound, tailkit.EventPortReleased:
        var change tailkit.PortEvent
        return json.Unmarshal(e.Data, &change)
    default:
        return nil
    }
})
```

Typed streaming helpers usually ignore unrelated events and decode only the event names they own.

---

## Tools

```go
tools, err := tailkit.Node(srv, "vps-1").Tools(ctx)
ok, err    := tailkit.Node(srv, "vps-1").HasTool(ctx, "devbox", "1.2.0")
// pass empty string for minVersion to match any version
```

---

## Files

```go
// fetch the node's files integration config (only paths with share=true are returned)
config, err := tailkit.Node(srv, "vps-1").Files().Config(ctx)

// read file content as string
content, err := tailkit.Node(srv, "vps-1").Files().Read(ctx, "/opt/myapp/compose.yml")

// stat a file — returns metadata including SHA-256 hash and size
stat, err := tailkit.Node(srv, "vps-1").Files().Stat(ctx, "/etc/nginx/nginx.conf")
// stat.SHA256  — hex-encoded SHA-256 of the file
// stat.Size    — file size in bytes
// stat.ModTime — last modified time

// download to a local path
err = tailkit.Node(srv, "vps-1").Files().Download(ctx,
    "/opt/myapp/compose.yml",
    "/local/dest/compose.yml",
)

// list directory
entries, err := tailkit.Node(srv, "vps-1").Files().List(ctx, "/opt/myapp/")

// push a local file
result, err := tailkit.Node(srv, "vps-1").Send(ctx, tailkit.SendRequest{
    LocalPath: "/home/user/nginx/api.conf",
    DestPath:  "/etc/nginx/conf.d/api.conf",
})

// push a local directory recursively
results, err := tailkit.Node(srv, "vps-1").SendDir(ctx, tailkit.SendDirRequest{
    LocalDir: "/home/user/nginx/",
    DestPath: "/etc/nginx/conf.d/",
})

// if a post-receive hook was triggered, poll it
if result.JobID != "" {
    hookResult, err := tailkit.Node(srv, "vps-1").ExecJob(ctx, result.JobID)
}
```

`SendDir` collects errors per file — one failed file does not abort remaining transfers.

### Fast hash check with Stat

`Files().Stat(ctx, path)` retrieves file metadata — including a SHA-256 hash and byte size — without returning the full file content. Use it for drift detection and integrity checks when transferring the content itself would be wasteful.

```go
stat, err := tailkit.Node(srv, "vps-1").Files().Stat(ctx, "/etc/nginx/nginx.conf")
if stat.SHA256 != vaultSHA256 {
    // file has drifted
}
```

---

## Vars

```go
vc := tailkit.Node(srv, "vps-1").Vars("myapp", "prod")

vars, err    := vc.List(ctx)
val, err     := vc.Get(ctx, "DATABASE_URL")
err           = vc.Set(ctx, "LOG_LEVEL", "debug")
err           = vc.Delete(ctx, "LOG_LEVEL")
envText, err := vc.Env(ctx)    // KEY=VALUE lines, sorted
```

### ExecWith

Fetch vars and inject them into a local subprocess without touching disk. Vars are overlaid on top of the current process environment; secrets disappear when the child exits.

```go
vars, err := tailkit.Node(srv, "vps-1").Vars("myapp", "prod").List(ctx)
err        = tailkit.ExecWith(ctx, vars, []string{"/usr/bin/node", "server.js"})
```

### Exec job streams

```go
job, err := tailkit.Node(srv, "vps-1").Docker().Start(ctx, "my-app")

err = tailkit.Node(srv, "vps-1").ExecJobStream(ctx, job.JobID, func(e tailkit.JobEvent) error {
    switch e.Event {
    case tailkit.EventJobStdout, tailkit.EventJobStderr:
        // e.Line
    case tailkit.EventJobStatus:
        // e.Status
    case tailkit.EventJobCompleted, tailkit.EventJobFailed:
        // e.ExitCode / e.Error
    }
    return nil
})
```

---

## Docker

```go
dc := tailkit.Node(srv, "vps-1").Docker()

// availability check — returns false on 503, never errors
available, _ := dc.Available(ctx)

// containers
containers, err := dc.Containers(ctx)
detail, err     := dc.Container(ctx, "my-app")
logs, err       := dc.Logs(ctx, "my-app", 100)
err             = dc.StreamLogs(ctx, "my-app", 100, func(line tailkit.LogLine) error {
    // line.Stream, line.Line, line.TS
    return nil
})
err             = dc.StreamStats(ctx, "my-app", func(stat container.StatsResponse) error {
    return nil
})
job, err        := dc.Start(ctx, "my-app")
job, err         = dc.Stop(ctx, "my-app")
job, err         = dc.Restart(ctx, "my-app")
job, err         = dc.Remove(ctx, "my-app")

// images
images, err := dc.Images(ctx)
job, err     = dc.Pull(ctx, "nginx:latest")

// compose
cc := dc.Compose()
projects, err := cc.Projects(ctx)
project, err  := cc.Project(ctx, "myapp")
job, err       = cc.Up(ctx, "myapp", "/opt/myapp/compose.yml")
job, err       = cc.Down(ctx, "myapp")
job, err       = cc.Pull(ctx, "myapp")
job, err       = cc.Restart(ctx, "myapp")
job, err       = cc.Build(ctx, "myapp")

// swarm (read-only)
sc := dc.Swarm()
nodes, err    := sc.Nodes(ctx)
services, err := sc.Services(ctx)
```

Container and image methods return Docker SDK types directly (`container.Summary`, `container.InspectResponse`, `image.Summary`, `swarm.Node`, `swarm.Service`). No translation layer between tailkitd and client.

---

## Systemd

```go
sc := tailkit.Node(srv, "vps-1").Systemd()

available, _ := sc.Available(ctx)

units, err   := sc.Units(ctx)
unit, err    := sc.Unit(ctx, "nginx.service")
file, err    := sc.UnitFile(ctx, "nginx.service")
job, err      = sc.Start(ctx, "nginx.service")
job, err      = sc.Stop(ctx, "nginx.service")
job, err      = sc.Restart(ctx, "nginx.service")
job, err      = sc.Reload(ctx, "nginx.service")
job, err      = sc.Enable(ctx, "nginx.service")
job, err      = sc.Disable(ctx, "nginx.service")
entries, err := sc.Journal(ctx, "nginx.service", 100)
entries, err  = sc.SystemJournal(ctx, 100)
err          = sc.StreamJournal(ctx, "nginx.service", 100, func(entry tailkit.JournalEntry) error {
    // entry.Message, entry.Unit, entry.Fields
    return nil
})
err          = sc.StreamSystemJournal(ctx, 100, func(entry tailkit.JournalEntry) error {
    return nil
})
```

---

## Metrics

```go
mc := tailkit.Node(srv, "vps-1").Metrics()

available, _   := mc.Available(ctx)
portsAvailable, err := mc.PortsAvailable(ctx)
host, err      := mc.Host(ctx)
cpu, err       := mc.CPU(ctx)
memory, err    := mc.Memory(ctx)
disks, err     := mc.Disk(ctx)
network, err   := mc.Network(ctx)
processes, err := mc.Processes(ctx) // []map[string]any snapshot endpoint
all, err       := mc.All(ctx)
ports, err     := mc.Ports(ctx)

err = mc.StreamCPU(ctx, func(stat []cpu.TimesStat) error {
    return nil
})
err = mc.StreamMemory(ctx, func(stat *mem.VirtualMemoryStat) error {
    return nil
})
err = mc.StreamNetwork(ctx, func(stat []net.IOCountersStat) error {
    return nil
})
err = mc.StreamProcesses(ctx, func(stat []tailkit.ProcessStat) error {
    return nil
})
err = mc.StreamAll(ctx, func(stat tailkit.AllMetrics) error {
    return nil
})
err = mc.StreamPorts(ctx, func(e tailkit.PortEvent) error {
    switch e.Kind {
    case "snapshot":
        // e.Ports
    case "bound", "released":
        // e.Port
    }
    return nil
})
```

Snapshot endpoints remain available alongside the stream helpers. `StreamPorts` emits one `snapshot` event on connect, then `bound` and `released` deltas. `StreamAll` decodes into `tailkit.AllMetrics`, which includes typed `CPU`, `Memory`, `Disk`, `Network`, `Processes`, and `Ports` sections when present.
