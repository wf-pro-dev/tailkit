package tailkit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/wf-pro-dev/tailkit/types"
)

const (
	EventJobStdout     = "job.stdout"
	EventJobStderr     = "job.stderr"
	EventJobStatus     = "job.status"
	EventJobCompleted  = "job.completed"
	EventJobFailed     = "job.failed"
	EventLogLine       = "log.line"
	EventStatsSnapshot = "stats.snapshot"
)

const (
	EventJournalEntry = "journal.entry"
	EventCPU          = "metrics.cpu"
	EventMemory       = "metrics.memory"
	EventNetwork      = "metrics.network"
	EventProcesses    = "metrics.processes"
	EventAll          = "metrics.all"
)

const (
	EventPortsSnapshot = "ports.snapshot"
	EventPortBound     = "port.bound"
	EventPortReleased  = "port.released"
)

// Event is one server-sent event received from a tailkitd stream.
type Event struct {
	Name string
	ID   int64
	Data json.RawMessage
}

func (n *NodeClient) Stream(ctx context.Context, path string, fn func(Event) error) error {
	return stream(ctx, n.httpClient(), func(lastID int64) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.baseURL()+path, nil)
		if err != nil {
			return nil, fmt.Errorf("tailkit: build request %s: %w", path, err)
		}
		req.Header.Set("Accept", "text/event-stream")
		if lastID > 0 {
			req.Header.Set("Last-Event-ID", strconv.FormatInt(lastID, 10))
		}
		return req, nil
	}, func(status int, body []byte) error {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errBody)
		return mapAPIError(status, path, errBody.Error)
	}, fn)
}

func stream(
	ctx context.Context,
	client *http.Client,
	buildReq func(lastID int64) (*http.Request, error),
	mapStatusError func(status int, body []byte) error,
	fn func(Event) error,
) error {
	var lastID int64

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := buildReq(lastID)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return mapStatusError(resp.StatusCode, body)
		}

		err = readSSE(resp.Body, func(e Event) error {
			if e.ID > 0 {
				lastID = e.ID
			}
			return fn(e)
		})
		resp.Body.Close()

		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
}

func readSSE(r io.Reader, fn func(Event) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		name      string
		id        int64
		hasID     bool
		dataLines []string
	)

	dispatch := func() error {
		if name == "" && !hasID && len(dataLines) == 0 {
			return nil
		}
		ev := Event{
			Name: name,
			Data: json.RawMessage(strings.Join(dataLines, "\n")),
		}
		if hasID {
			ev.ID = id
		}
		name = ""
		id = 0
		hasID = false
		dataLines = dataLines[:0]
		return fn(ev)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimPrefix(value, " ")

		switch field {
		case "event":
			name = value
		case "data":
			dataLines = append(dataLines, value)
		case "id":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}
			id = n
			hasID = true
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return dispatch()
}

func decodeStreamError(e Event) error {
	if e.Name != "error" {
		return nil
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(e.Data, &payload); err != nil {
		return fmt.Errorf("tailkit: decode stream error: %w", err)
	}
	if payload.Error == "" {
		payload.Error = "remote stream error"
	}
	return fmt.Errorf("tailkit: %s", payload.Error)
}

func (n *NodeClient) ExecJobStream(ctx context.Context, jobID string, fn func(types.JobEvent) error) error {
	path := "/exec/jobs/" + url.PathEscape(jobID) + "?stream=true"
	return n.Stream(ctx, path, func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		switch e.Name {
		case EventJobStdout, EventJobStderr, EventJobStatus, EventJobCompleted, EventJobFailed:
			var out types.JobEvent
			if err := json.Unmarshal(e.Data, &out); err != nil {
				return fmt.Errorf("tailkit: decode exec job stream event %q: %w", e.Name, err)
			}
			out.Event = e.Name
			return fn(out)
		default:
			return nil
		}
	})
}

func (dc *DockerClient) StreamLogs(ctx context.Context, containerID string, tail int, fn func(types.LogLine) error) error {
	path := fmt.Sprintf("%s/containers/%s/logs?tail=%d&follow=true", dockerBase, url.PathEscape(containerID), tail)
	return dc.node.Stream(ctx, path, func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventLogLine {
			return nil
		}
		var out types.LogLine
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode docker log stream event: %w", err)
		}
		return fn(out)
	})
}

func (dc *DockerClient) StreamStats(ctx context.Context, containerID string, fn func(container.StatsResponse) error) error {
	path := fmt.Sprintf("%s/containers/%s/stats", dockerBase, url.PathEscape(containerID))
	return dc.node.Stream(ctx, path, func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventStatsSnapshot {
			return nil
		}
		var out container.StatsResponse
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode docker stats stream event: %w", err)
		}
		return fn(out)
	})
}

func (sc *SystemdClient) StreamJournal(ctx context.Context, unit string, lines int, fn func(types.JournalEntry) error) error {
	path := fmt.Sprintf("%s/units/%s/journal?lines=%d&follow=true", systemdBase, url.PathEscape(unit), lines)
	return sc.node.Stream(ctx, path, func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventJournalEntry {
			return nil
		}
		var out types.JournalEntry
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode journal stream event: %w", err)
		}
		return fn(out)
	})
}

func (sc *SystemdClient) StreamSystemJournal(ctx context.Context, lines int, fn func(types.JournalEntry) error) error {
	path := fmt.Sprintf("%s/journal?lines=%d&follow=true", systemdBase, lines)
	return sc.node.Stream(ctx, path, func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventJournalEntry {
			return nil
		}
		var out types.JournalEntry
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode system journal stream event: %w", err)
		}
		return fn(out)
	})
}

func (mc *MetricsClient) PortsAvailable(ctx context.Context) (bool, error) {
	var resp map[string]bool
	if err := mc.node.do(ctx, http.MethodGet, metricsBase+"/ports/available", nil, &resp); err != nil {
		return false, err
	}
	return resp["available"], nil
}

func (mc *MetricsClient) Ports(ctx context.Context) ([]types.ListenPort, error) {
	var out []types.ListenPort
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/ports", nil, &out)
}

func (mc *MetricsClient) StreamCPU(ctx context.Context, fn func([]cpu.TimesStat) error) error {
	return mc.node.Stream(ctx, metricsBase+"/cpu/stream", func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventCPU {
			return nil
		}
		var out []cpu.TimesStat
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode CPU stream event: %w", err)
		}
		return fn(out)
	})
}

func (mc *MetricsClient) StreamMemory(ctx context.Context, fn func(*mem.VirtualMemoryStat) error) error {
	return mc.node.Stream(ctx, metricsBase+"/memory/stream", func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventMemory {
			return nil
		}
		var out mem.VirtualMemoryStat
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode memory stream event: %w", err)
		}
		return fn(&out)
	})
}

func (mc *MetricsClient) StreamNetwork(ctx context.Context, fn func([]gopsnet.IOCountersStat) error) error {
	return mc.node.Stream(ctx, metricsBase+"/network/stream", func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventNetwork {
			return nil
		}
		var out []gopsnet.IOCountersStat
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode network stream event: %w", err)
		}
		return fn(out)
	})
}

func (mc *MetricsClient) StreamProcesses(ctx context.Context, fn func([]types.ProcessStat) error) error {
	return mc.node.Stream(ctx, metricsBase+"/processes/stream", func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventProcesses {
			return nil
		}
		var out []types.ProcessStat
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode processes stream event: %w", err)
		}
		return fn(out)
	})
}

func (mc *MetricsClient) StreamAll(ctx context.Context, fn func(types.AllMetrics) error) error {
	return mc.node.Stream(ctx, metricsBase+"/all/stream", func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		if e.Name != EventAll {
			return nil
		}
		var out types.AllMetrics
		if err := json.Unmarshal(e.Data, &out); err != nil {
			return fmt.Errorf("tailkit: decode all-metrics stream event: %w", err)
		}
		return fn(out)
	})
}

func (mc *MetricsClient) StreamPorts(ctx context.Context, fn func(types.PortEvent) error) error {
	return mc.node.Stream(ctx, metricsBase+"/ports/stream", func(e Event) error {
		if err := decodeStreamError(e); err != nil {
			return err
		}
		switch e.Name {
		case EventPortsSnapshot, EventPortBound, EventPortReleased:
			var out types.PortEvent
			if err := json.Unmarshal(e.Data, &out); err != nil {
				return fmt.Errorf("tailkit: decode ports stream event %q: %w", e.Name, err)
			}
			if out.Kind == "" {
				switch e.Name {
				case EventPortsSnapshot:
					out.Kind = "snapshot"
				case EventPortBound:
					out.Kind = "bound"
				case EventPortReleased:
					out.Kind = "released"
				}
			}
			return fn(out)
		default:
			return nil
		}
	})
}
