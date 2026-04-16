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
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/wf-pro-dev/tailkit/types"
)

const (
	EventError         = types.EventError
	EventJobStdout     = types.EventJobStdout
	EventJobStderr     = types.EventJobStderr
	EventJobStatus     = types.EventJobStatus
	EventJobCompleted  = types.EventJobCompleted
	EventJobFailed     = types.EventJobFailed
	EventLogLine       = types.EventLogLine
	EventStatsSnapshot = types.EventStatsSnapshot
	EventJournalEntry  = types.EventJournalEntry
	EventCPU           = types.EventCPU
	EventMemory        = types.EventMemory
	EventNetwork       = types.EventNetwork
	EventProcesses     = types.EventProcesses
	EventAll           = types.EventAll
	EventPortsSnapshot = types.EventPortsSnapshot
	EventPortBound     = types.EventPortBound
	EventPortReleased  = types.EventPortReleased
)

func Stream[T any](ctx context.Context, node *NodeClient, path string, names []string, fn func(types.Event[T]) error) error {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}

	return stream(ctx, node.httpClient(), func(lastID int64) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, node.baseURL()+path, nil)
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
	}, func(raw types.RawEvent) error {
		rawEvent, err := DecodeEvent[json.RawMessage](raw)
		if err != nil {
			return fmt.Errorf("tailkit: %w", err)
		}
		if err := decodeStreamError(rawEvent); err != nil {
			return err
		}
		if _, ok := allowed[raw.Name]; !ok {
			return nil
		}
		event, err := DecodeEvent[T](raw)
		if err != nil {
			return err
		}
		return fn(event)
	})
}

func stream(
	ctx context.Context,
	client *http.Client,
	buildReq func(lastID int64) (*http.Request, error),
	mapStatusError func(status int, body []byte) error,
	fn func(types.RawEvent) error,
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

		err = readSSE(resp.Body, func(e types.RawEvent) error {
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

func DecodeEvent[T any](raw types.RawEvent) (types.Event[T], error) {
	var data T
	if len(raw.Data) > 0 {
		if err := json.Unmarshal(raw.Data, &data); err != nil {
			return Event[T]{}, fmt.Errorf("decode event %q: %w", raw.Name, err)
		}
	}
	return Event[T]{
		Name: raw.Name,
		ID:   raw.ID,
		Data: data,
	}, nil
}

func readSSE(r io.Reader, fn func(types.RawEvent) error) error {
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
		ev := types.RawEvent{
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

func decodeStreamError[T any](e types.Event[T]) error {
	if e.Name != EventError {
		return nil
	}
	switch data := any(e.Data).(type) {
	case json.RawMessage:
		var payload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("tailkit: decode stream error: %w", err)
		}
		if payload.Error == "" {
			payload.Error = "remote stream error"
		}
		return fmt.Errorf("tailkit: %s", payload.Error)
	default:
		return nil
	}
}

func (n *NodeClient) ExecJobStream(ctx context.Context, jobID string, fn func(types.Event[types.JobUpdate]) error) error {
	path := "/exec/jobs/" + url.PathEscape(jobID) + "?stream=true"
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
	}, func(raw types.RawEvent) error {
		rawEvent, err := DecodeEvent[json.RawMessage](raw)
		if err != nil {
			return fmt.Errorf("tailkit: %w", err)
		}
		if err := decodeStreamError(rawEvent); err != nil {
			return err
		}
		switch raw.Name {
		case EventJobStdout, EventJobStderr, EventJobStatus, EventJobCompleted, EventJobFailed:
			event, err := DecodeEvent[types.JobUpdate](raw)
			if err != nil {
				return err
			}
			event.Data.Event = event.Name
			return fn(event)
		default:
			return nil
		}
	})
}

func (dc *DockerClient) StreamLogs(ctx context.Context, containerID string, tail int, fn func(types.Event[types.LogLine]) error) error {
	path := fmt.Sprintf("%s/containers/%s/logs?tail=%d&follow=true", dockerBase, url.PathEscape(containerID), tail)
	return Stream(ctx, dc.node, path, []string{EventLogLine}, fn)
}

func (dc *DockerClient) StreamStats(ctx context.Context, containerID string, fn func(types.Event[container.StatsResponse]) error) error {
	path := fmt.Sprintf("%s/containers/%s/stats", dockerBase, url.PathEscape(containerID))
	return Stream(ctx, dc.node, path, []string{EventStatsSnapshot}, fn)
}

func (sc *SystemdClient) StreamJournal(ctx context.Context, unit string, lines int, fn func(types.Event[types.JournalEntry]) error) error {
	path := fmt.Sprintf("%s/units/%s/journal?lines=%d&follow=true", systemdBase, url.PathEscape(unit), lines)
	return Stream(ctx, sc.node, path, []string{EventJournalEntry}, fn)
}

func (sc *SystemdClient) StreamSystemJournal(ctx context.Context, lines int, fn func(types.Event[types.JournalEntry]) error) error {
	path := fmt.Sprintf("%s/journal?lines=%d&follow=true", systemdBase, lines)
	return Stream(ctx, sc.node, path, []string{EventJournalEntry}, fn)
}

func (mc *MetricsClient) PortsAvailable(ctx context.Context) (bool, error) {
	var resp map[string]bool
	if err := mc.node.do(ctx, http.MethodGet, metricsBase+"/ports/available", nil, &resp); err != nil {
		return false, err
	}
	return resp["available"], nil
}

func (mc *MetricsClient) Ports(ctx context.Context) ([]types.Port, error) {
	var out []types.Port
	return out, mc.node.do(ctx, http.MethodGet, metricsBase+"/ports", nil, &out)
}

func (mc *MetricsClient) StreamCPU(ctx context.Context, fn func(types.Event[types.CPU]) error) error {
	return Stream(ctx, mc.node, metricsBase+"/cpu/stream", []string{EventCPU}, fn)
}

func (mc *MetricsClient) StreamMemory(ctx context.Context, fn func(types.Event[types.Memory]) error) error {
	return Stream(ctx, mc.node, metricsBase+"/memory/stream", []string{EventMemory}, fn)
}

func (mc *MetricsClient) StreamNetwork(ctx context.Context, fn func(types.Event[[]gopsnet.IOCountersStat]) error) error {
	return Stream(ctx, mc.node, metricsBase+"/network/stream", []string{EventNetwork}, fn)
}

func (mc *MetricsClient) StreamProcesses(ctx context.Context, fn func(types.Event[[]types.Process]) error) error {
	return Stream(ctx, mc.node, metricsBase+"/processes/stream", []string{EventProcesses}, fn)
}

func (mc *MetricsClient) StreamAll(ctx context.Context, fn func(types.Event[types.Metrics]) error) error {
	return Stream(ctx, mc.node, metricsBase+"/all/stream", []string{EventAll}, fn)
}

func (mc *MetricsClient) StreamPorts(ctx context.Context, fn func(types.Event[types.PortUpdate]) error) error {
	return Stream(ctx, mc.node, metricsBase+"/ports/stream", []string{EventPortsSnapshot, EventPortBound, EventPortReleased}, func(event types.Event[types.PortUpdate]) error {
		if event.Data.Kind == "" {
			switch event.Name {
			case EventPortsSnapshot:
				event.Data.Kind = "snapshot"
			case EventPortBound:
				event.Data.Kind = "bound"
			case EventPortReleased:
				event.Data.Kind = "released"
			}
		}
		return fn(event)
	})
}
