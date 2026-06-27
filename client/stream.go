package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// StreamEvents reads SSE from the body and unmarshals them into the requested type T.
func StreamEvents[T any](ctx context.Context, body io.ReadCloser, allowedEvents []string, fn func(Event[T]) error) error {
	defer body.Close()

	allowed := make(map[string]struct{}, len(allowedEvents))
	for _, name := range allowedEvents {
		allowed[name] = struct{}{}
	}

	return readSSE(body, func(raw RawEvent) error {
		// Check for context cancellation
		if err := ctx.Err(); err != nil {
			return err
		}

		// Decode the raw JSON to check for stream errors
		rawEvent, err := DecodeEvent[json.RawMessage](raw)
		if err != nil {
			return fmt.Errorf("tailkit: %w", err)
		}
		if err := decodeStreamError(rawEvent); err != nil {
			return err
		}

		// Filter events if an allowed list is provided
		if len(allowed) > 0 {
			if _, ok := allowed[raw.Name]; !ok {
				return nil
			}
		}

		// Decode into the strongly typed struct T
		event, err := DecodeEvent[T](raw)
		if err != nil {
			return err
		}
		return fn(event)
	})
}

func readSSE(r io.Reader, fn func(RawEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var name string
	var id int64
	var hasID bool
	var dataLines []string

	dispatch := func() error {
		if name == "" && !hasID && len(dataLines) == 0 {
			return nil
		}
		ev := RawEvent{
			Name: name,
			Data: json.RawMessage(strings.Join(dataLines, "\n")),
		}
		if hasID {
			ev.ID = id
		}
		name, id, hasID, dataLines = "", 0, false, dataLines[:0]
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
		} // SSE Comment

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
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				id = n
				hasID = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return dispatch()
}

func decodeStreamError[T any](e Event[T]) error {
	if e.Name != "error" { // Assuming EventError constant
		return nil
	}
	if data, ok := any(e.Data).(json.RawMessage); ok {
		var payload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &payload); err == nil && payload.Error != "" {
			return fmt.Errorf("tailkit: %s", payload.Error)
		}
	}
	return fmt.Errorf("tailkit: remote stream error")
}

func DecodeEvent[T any](raw RawEvent) (Event[T], error) {
	var data T
	if len(raw.Data) > 0 {
		if err := json.Unmarshal(raw.Data, &data); err != nil {
			return Event[T]{}, fmt.Errorf("decode event %q: %w", raw.Name, err)
		}
	}
	return Event[T]{Name: raw.Name, ID: raw.ID, Data: data}, nil
}
