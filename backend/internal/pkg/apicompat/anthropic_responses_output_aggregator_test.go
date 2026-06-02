package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runAggregated drives a sequence of Anthropic stream events through the
// upstream converter and the fork's output aggregator, returning every patched
// Responses event in order.
func runAggregated(t *testing.T, in []AnthropicStreamEvent) []ResponsesStreamEvent {
	t.Helper()
	state := NewAnthropicEventToResponsesState()
	agg := NewResponsesOutputAggregator()
	var out []ResponsesStreamEvent
	for i := range in {
		out = append(out, agg.Patch(AnthropicEventToResponsesEvents(&in[i], state))...)
	}
	out = append(out, agg.Patch(FinalizeAnthropicResponsesStream(state))...)
	return out
}

func findResponsesEvent(events []ResponsesStreamEvent, typ string) *ResponsesStreamEvent {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}
	return nil
}

// runWithTerminalGuarantee mirrors the handler's finalizeStream: feed events,
// finalize, then apply the EnsureTerminalEvents fallback.
func runWithTerminalGuarantee(t *testing.T, in []AnthropicStreamEvent) []ResponsesStreamEvent {
	t.Helper()
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-test"
	agg := NewResponsesOutputAggregator()
	var out []ResponsesStreamEvent
	for i := range in {
		out = append(out, agg.Patch(AnthropicEventToResponsesEvents(&in[i], state))...)
	}
	out = append(out, agg.Patch(FinalizeAnthropicResponsesStream(state))...)
	out = append(out, agg.EnsureTerminalEvents(state.ResponseID, state.Model)...)
	return out
}

// Regression: an upstream stream that ends before message_start (e.g. an
// `error` event the converter drops) produces no terminal event from the
// converter; EnsureTerminalEvents must synthesize response.created +
// response.completed so Codex doesn't fail with "stream closed before
// response.completed".
func TestResponsesOutputAggregator_TerminalGuaranteeOnAbnormalEnd(t *testing.T) {
	// No recognized events at all (converter emits nothing).
	events := runWithTerminalGuarantee(t, []AnthropicStreamEvent{
		{Type: "error"}, // dropped by the converter
	})

	created := findResponsesEvent(events, "response.created")
	require.NotNil(t, created, "synthetic response.created must be emitted")
	completed := findResponsesEvent(events, "response.completed")
	require.NotNil(t, completed, "synthetic response.completed must be emitted")
	require.NotNil(t, completed.Response)
	assert.Equal(t, "completed", completed.Response.Status)
	assert.NotNil(t, completed.Response.Output) // non-nil, marshals as []
	assert.Equal(t, "claude-test", completed.Response.Model)
	// created must precede completed.
	var ci, fi = -1, -1
	for i := range events {
		if events[i].Type == "response.created" {
			ci = i
		}
		if events[i].Type == "response.completed" {
			fi = i
		}
	}
	assert.Less(t, ci, fi, "response.created must come before response.completed")
}

// When the stream completed normally, EnsureTerminalEvents must be a no-op (no
// duplicate terminal events).
func TestResponsesOutputAggregator_TerminalGuaranteeNoDoubleOnNormalEnd(t *testing.T) {
	events := runWithTerminalGuarantee(t, []AnthropicStreamEvent{
		{Type: "message_start", Message: &AnthropicResponse{ID: "msg_ok", Model: "claude-test"}},
		{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "text"}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "hi"}},
		{Type: "content_block_stop"},
		{Type: "message_stop"},
	})

	createdCount, completedCount := 0, 0
	for i := range events {
		switch events[i].Type {
		case "response.created":
			createdCount++
		case "response.completed":
			completedCount++
		}
	}
	assert.Equal(t, 1, createdCount, "exactly one response.created")
	assert.Equal(t, 1, completedCount, "exactly one response.completed (no duplicate from fallback)")
}

// Regression: the streamed text deltas must be aggregated back into the
// terminal events. Clients like Codex treat response.completed.output as
// authoritative, so an empty output[] (or missing item.content on
// output_item.done) makes them render nothing despite the deltas.
func TestResponsesOutputAggregator_TextAggregatedIntoTerminalEvents(t *testing.T) {
	events := runAggregated(t, []AnthropicStreamEvent{
		{Type: "message_start", Message: &AnthropicResponse{ID: "msg_text", Model: "claude-sonnet-4-6"}},
		{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "text"}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "Hello"}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: ", "}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "world"}},
		{Type: "content_block_stop"},
		{Type: "message_stop"},
	})

	textDone := findResponsesEvent(events, "response.output_text.done")
	require.NotNil(t, textDone, "response.output_text.done must be emitted")
	assert.Equal(t, "Hello, world", textDone.Text)

	itemDone := findResponsesEvent(events, "response.output_item.done")
	require.NotNil(t, itemDone, "response.output_item.done must be emitted")
	require.NotNil(t, itemDone.Item)
	assert.Equal(t, "message", itemDone.Item.Type)
	assert.Equal(t, "assistant", itemDone.Item.Role)
	require.Len(t, itemDone.Item.Content, 1)
	assert.Equal(t, "output_text", itemDone.Item.Content[0].Type)
	assert.Equal(t, "Hello, world", itemDone.Item.Content[0].Text)

	completed := findResponsesEvent(events, "response.completed")
	require.NotNil(t, completed, "response.completed must be emitted")
	require.NotNil(t, completed.Response)
	require.Len(t, completed.Response.Output, 1)
	msg := completed.Response.Output[0]
	assert.Equal(t, "message", msg.Type)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, "Hello, world", msg.Content[0].Text)
}

// Regression: function_call arguments must likewise survive into the terminal
// events, not just the streamed deltas.
func TestResponsesOutputAggregator_ToolCallAggregatedIntoTerminalEvents(t *testing.T) {
	events := runAggregated(t, []AnthropicStreamEvent{
		{Type: "message_start", Message: &AnthropicResponse{ID: "msg_tool", Model: "claude-sonnet-4-6"}},
		{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "toolu_1", Name: "get_weather"}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"city"`}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `:"NYC"}`}},
		{Type: "content_block_stop"},
		{Type: "message_stop"},
	})

	argsDone := findResponsesEvent(events, "response.function_call_arguments.done")
	require.NotNil(t, argsDone, "response.function_call_arguments.done must be emitted")
	assert.JSONEq(t, `{"city":"NYC"}`, argsDone.Arguments)

	itemDone := findResponsesEvent(events, "response.output_item.done")
	require.NotNil(t, itemDone)
	require.NotNil(t, itemDone.Item)
	assert.Equal(t, "function_call", itemDone.Item.Type)
	assert.Equal(t, "get_weather", itemDone.Item.Name)
	assert.JSONEq(t, `{"city":"NYC"}`, itemDone.Item.Arguments)

	completed := findResponsesEvent(events, "response.completed")
	require.NotNil(t, completed)
	require.Len(t, completed.Response.Output, 1)
	assert.Equal(t, "function_call", completed.Response.Output[0].Type)
	assert.JSONEq(t, `{"city":"NYC"}`, completed.Response.Output[0].Arguments)
}

// A tool call following a text block must yield two completed output items in
// order, each with its own payload.
func TestResponsesOutputAggregator_TextThenToolCall(t *testing.T) {
	events := runAggregated(t, []AnthropicStreamEvent{
		{Type: "message_start", Message: &AnthropicResponse{ID: "msg_mix", Model: "claude-sonnet-4-6"}},
		{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "text"}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "Let me check."}},
		{Type: "content_block_stop"},
		{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "toolu_2", Name: "search"}},
		{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"q":"go"}`}},
		{Type: "content_block_stop"},
		{Type: "message_stop"},
	})

	completed := findResponsesEvent(events, "response.completed")
	require.NotNil(t, completed)
	require.Len(t, completed.Response.Output, 2)

	assert.Equal(t, "message", completed.Response.Output[0].Type)
	require.Len(t, completed.Response.Output[0].Content, 1)
	assert.Equal(t, "Let me check.", completed.Response.Output[0].Content[0].Text)

	assert.Equal(t, "function_call", completed.Response.Output[1].Type)
	assert.Equal(t, "search", completed.Response.Output[1].Name)
	assert.JSONEq(t, `{"q":"go"}`, completed.Response.Output[1].Arguments)
}
