package apicompat

// Fork addition (CLAUDE.md §5): kept in a separate file so the upstream
// converter (anthropic_to_responses_response.go) stays byte-identical and
// merges from upstream remain conflict-free.
//
// The upstream Anthropic→Responses streaming converter forwards the streaming
// deltas correctly but leaves the *terminal* events without their payload:
//   - response.output_text.done            → no text
//   - response.reasoning_summary_text.done → no text
//   - response.function_call_arguments.done → no arguments
//   - response.output_item.done            → item without content/arguments
//   - response.completed                   → empty output[]
//
// Clients that treat response.completed.output as the authoritative result
// (e.g. Codex) then render nothing despite having received every delta. This
// aggregator re-accumulates the deltas it sees and back-fills those terminal
// events in place. It is a no-op for fields the converter already populated, so
// it is safe to apply even if upstream later starts filling them itself.

// ResponsesOutputAggregator re-aggregates the streamed deltas of a single
// Anthropic→Responses conversion and patches the terminal events that the
// upstream converter leaves empty. Pair one aggregator with one converter state
// and pipe every produced batch (including the finalize batch) through Patch.
type ResponsesOutputAggregator struct {
	// Per-item accumulators, reset when a new output item starts.
	text    string
	args    string
	summary string
	callID  string
	name    string

	// completed collects finished output items in order for response.completed.
	completed []ResponsesOutput
}

// NewResponsesOutputAggregator returns a ready-to-use aggregator.
func NewResponsesOutputAggregator() *ResponsesOutputAggregator {
	return &ResponsesOutputAggregator{}
}

// Patch enriches a batch of converter-produced events in place and returns the
// same slice. Call it on every batch from AnthropicEventToResponsesEvents and
// FinalizeAnthropicResponsesStream, in order.
func (a *ResponsesOutputAggregator) Patch(events []ResponsesStreamEvent) []ResponsesStreamEvent {
	for i := range events {
		e := &events[i]
		switch e.Type {
		case "response.output_item.added":
			a.startItem(e.Item)
		case "response.output_text.delta":
			a.text += e.Delta
		case "response.reasoning_summary_text.delta":
			a.summary += e.Delta
		case "response.function_call_arguments.delta":
			a.args += e.Delta
			a.captureCall(e.CallID, e.Name)
		case "response.output_text.done":
			if e.Text == "" {
				e.Text = a.text
			}
		case "response.reasoning_summary_text.done":
			if e.Text == "" {
				e.Text = a.summary
			}
		case "response.function_call_arguments.done":
			a.captureCall(e.CallID, e.Name)
			if e.Arguments == "" {
				e.Arguments = a.argsOrEmptyObject()
			}
		case "response.output_item.done":
			a.finishItem(e)
		case "response.completed":
			if e.Response != nil {
				e.Response.Output = a.outputs()
			}
		}
	}
	return events
}

// startItem resets the per-item accumulators and captures identity for a
// function_call (its call_id/name are absent from the later output_item.done).
func (a *ResponsesOutputAggregator) startItem(item *ResponsesOutput) {
	a.text, a.args, a.summary, a.callID, a.name = "", "", "", "", ""
	if item != nil {
		a.callID = item.CallID
		a.name = item.Name
	}
}

func (a *ResponsesOutputAggregator) captureCall(callID, name string) {
	if callID != "" {
		a.callID = callID
	}
	if name != "" {
		a.name = name
	}
}

// finishItem fills the closing item with its accumulated payload, records a copy
// for the eventual response.completed, then clears the per-item accumulators.
func (a *ResponsesOutputAggregator) finishItem(e *ResponsesStreamEvent) {
	if e.Item == nil {
		a.startItem(nil)
		return
	}
	switch e.Item.Type {
	case "message":
		if e.Item.Role == "" {
			e.Item.Role = "assistant"
		}
		if len(e.Item.Content) == 0 {
			e.Item.Content = []ResponsesContentPart{{Type: "output_text", Text: a.text}}
		}
	case "function_call":
		if e.Item.CallID == "" {
			e.Item.CallID = a.callID
		}
		if e.Item.Name == "" {
			e.Item.Name = a.name
		}
		if e.Item.Arguments == "" {
			e.Item.Arguments = a.argsOrEmptyObject()
		}
	case "reasoning":
		if len(e.Item.Summary) == 0 && a.summary != "" {
			e.Item.Summary = []ResponsesSummary{{Type: "summary_text", Text: a.summary}}
		}
	}
	a.completed = append(a.completed, *e.Item)
	a.startItem(nil)
}

func (a *ResponsesOutputAggregator) argsOrEmptyObject() string {
	if a.args == "" {
		return "{}"
	}
	return a.args
}

// outputs returns the collected items, never nil so response.completed.output
// serialises as [] rather than null.
func (a *ResponsesOutputAggregator) outputs() []ResponsesOutput {
	if a.completed == nil {
		return []ResponsesOutput{}
	}
	return a.completed
}
