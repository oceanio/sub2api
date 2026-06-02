package service

import (
	"bufio"
	"strings"
	"testing"
)

func collectUpstreamSSE(raw string) [][2]string {
	sc := bufio.NewScanner(strings.NewReader(raw))
	var out [][2]string
	forEachUpstreamSSEEvent(sc, func(eventType, data string) bool {
		out = append(out, [2]string{eventType, data})
		return false
	})
	return out
}

// The SSE parser must be field-order independent: `event:`-first (standard
// Anthropic) and `data:`-first (some upstreams, e.g. llmgw) must yield the same
// events. The old parser dropped every event for the `data:`-first ordering,
// producing an empty Responses output.
func TestForEachUpstreamSSEEvent_FieldOrderIndependent(t *testing.T) {
	eventFirst := "event: message_start\n" +
		`data: {"type":"message_start"}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","delta":{"text":"Hi"}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	dataFirst := `data: {"type":"message_start"}` + "\n" +
		"event: message_start\n\n" +
		`data: {"type":"content_block_delta","delta":{"text":"Hi"}}` + "\n" +
		"event: content_block_delta\n\n" +
		`data: {"type":"message_stop"}` + "\n" +
		"event: message_stop\n\n"

	want := [][2]string{
		{"message_start", `{"type":"message_start"}`},
		{"content_block_delta", `{"type":"content_block_delta","delta":{"text":"Hi"}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"event_first", eventFirst},
		{"data_first", dataFirst},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := collectUpstreamSSE(tc.raw)
			if len(got) != len(want) {
				t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("event %d = %+v, want %+v", i, got[i], want[i])
				}
			}
		})
	}
}

// A trailing event without a blank line before EOF must still be dispatched.
func TestForEachUpstreamSSEEvent_TrailingEventNoBlankLine(t *testing.T) {
	raw := "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n" // no trailing blank line
	got := collectUpstreamSSE(raw)
	if len(got) != 1 || got[0][0] != "message_stop" {
		t.Fatalf("trailing event not dispatched: %+v", got)
	}
}

// fn returning true must stop iteration early (client disconnect).
func TestForEachUpstreamSSEEvent_EarlyStop(t *testing.T) {
	raw := "event: a\ndata: {}\n\nevent: b\ndata: {}\n\nevent: c\ndata: {}\n\n"
	sc := bufio.NewScanner(strings.NewReader(raw))
	count := 0
	forEachUpstreamSSEEvent(sc, func(eventType, data string) bool {
		count++
		return eventType == "b" // stop after the second event
	})
	if count != 2 {
		t.Fatalf("expected to stop after 2 events, processed %d", count)
	}
}
