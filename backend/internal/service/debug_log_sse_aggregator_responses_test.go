package service

import (
	"encoding/json"
	"testing"
)

// A Responses API stream (response.* events) routed through the OpenAI protocol
// must be aggregated into the final response object — not the empty
// {"choices":[],"object":"chat.completion"} the chat aggregator produces.
func TestAggregateStream_OpenAIResponsesStream(t *testing.T) {
	raw := "" +
		"event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_1","object":"response","model":"claude-sonnet-4-6","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"item_1","role":"assistant","status":"in_progress"}}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"item_id":"item_1","delta":"Hello"}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"item_id":"item_1","delta":", world"}` + "\n\n" +
		"event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"item_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello, world"}]}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"claude-sonnet-4-6","status":"completed","output":[{"type":"message","id":"item_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello, world"}]}],"usage":{"input_tokens":10,"output_tokens":3}}}` + "\n\n"

	out, err := AggregateStream(DebugLogProtocolOpenAI, []byte(raw))
	if err != nil {
		t.Fatalf("AggregateStream error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("aggregated body is not valid JSON: %v\n%s", err, out)
	}
	if got["object"] != "response" {
		t.Fatalf("expected object=response, got %v (body=%s)", got["object"], out)
	}
	output, ok := got["output"].([]any)
	if !ok || len(output) != 1 {
		t.Fatalf("expected 1 output item, got %v (body=%s)", got["output"], out)
	}
	msg := output[0].(map[string]any)
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	if part["text"] != "Hello, world" {
		t.Fatalf("expected aggregated text 'Hello, world', got %q (body=%s)", part["text"], out)
	}
	if usage, ok := got["usage"].(map[string]any); !ok || usage["output_tokens"] == nil {
		t.Fatalf("expected usage carried from response.completed, got %v", got["usage"])
	}
}

// When the stream is truncated before response.completed, output must still be
// reconstructed from the incremental deltas.
func TestAggregateStream_OpenAIResponsesTruncatedBeforeCompleted(t *testing.T) {
	raw := "" +
		"event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_2","object":"response","model":"m","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"item_2","role":"assistant","status":"in_progress"}}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"partial"}` + "\n\n"

	out, err := AggregateStream(DebugLogProtocolOpenAI, []byte(raw))
	if err != nil {
		t.Fatalf("AggregateStream error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	output := got["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("expected reconstructed output, got %v (body=%s)", output, out)
	}
	part := output[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if part["text"] != "partial" {
		t.Fatalf("expected reconstructed text 'partial', got %q", part["text"])
	}
}

// A function_call in a Responses stream must aggregate its streamed arguments.
func TestAggregateStream_OpenAIResponsesFunctionCall(t *testing.T) {
	raw := "" +
		"event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"r","object":"response","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather"}}` + "\n\n" +
		"event: response.function_call_arguments.delta\n" +
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\""}` + "\n\n" +
		"event: response.function_call_arguments.delta\n" +
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":":\"NYC\"}"}` + "\n\n"

	out, err := AggregateStream(DebugLogProtocolOpenAI, []byte(raw))
	if err != nil {
		t.Fatalf("AggregateStream error: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	item := got["output"].([]any)[0].(map[string]any)
	if item["name"] != "get_weather" {
		t.Fatalf("expected name get_weather, got %v", item["name"])
	}
	if item["arguments"] != `{"city":"NYC"}` {
		t.Fatalf("expected aggregated arguments, got %q (body=%s)", item["arguments"], out)
	}
}

// Regression guard: a genuine Chat Completions stream must still aggregate into
// a chat.completion with populated choices (the dispatcher must not misroute).
func TestAggregateStream_OpenAIChatCompletionsStillWorks(t *testing.T) {
	raw := "" +
		`data: {"object":"chat.completion.chunk","id":"c1","model":"gpt","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"}}]}` + "\n\n" +
		`data: {"object":"chat.completion.chunk","id":"c1","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	out, err := AggregateStream(DebugLogProtocolOpenAI, []byte(raw))
	if err != nil {
		t.Fatalf("AggregateStream error: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	if got["object"] != "chat.completion" {
		t.Fatalf("expected chat.completion, got %v", got["object"])
	}
	choices := got["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("expected 1 choice, got %d (body=%s)", len(choices), out)
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Hi there" {
		t.Fatalf("expected aggregated content 'Hi there', got %q", msg["content"])
	}
}
