package service

import (
	"bufio"
	"bytes"
	"errors"
	"strings"

	// sonic 用于加速 SSE 事件 JSON 解析，drop-in 替代 encoding/json 的
	// Marshal/Unmarshal（debug-log 内部使用，主链路仍用 encoding/json）。
	sonic "github.com/bytedance/sonic"
)

// DebugLogProtocol identifies the upstream protocol of a captured response,
// used to pick the right SSE aggregator for streaming bodies.
type DebugLogProtocol string

const (
	DebugLogProtocolAnthropic DebugLogProtocol = "anthropic"
	DebugLogProtocolOpenAI    DebugLogProtocol = "openai"
)

// AggregateStream parses raw SSE bytes and rebuilds the final JSON response
// according to the upstream protocol. Returns ErrSSEAggregationUnsupported if
// the protocol is unknown.
func AggregateStream(protocol DebugLogProtocol, raw []byte) ([]byte, error) {
	switch protocol {
	case DebugLogProtocolAnthropic:
		return aggregateWith(raw, newAnthropicAggregator())
	case DebugLogProtocolOpenAI:
		return aggregateWith(raw, newOpenAIAggregator())
	}
	return nil, ErrSSEAggregationUnsupported
}

// ErrSSEAggregationUnsupported indicates the caller passed a protocol value
// that no aggregator handles. Caller should fall back to storing raw text.
var ErrSSEAggregationUnsupported = errors.New("debug log sse aggregation unsupported")

func aggregateWith(raw []byte, agg sseAggregator) ([]byte, error) {
	if err := feedSSE(raw, agg); err != nil {
		return nil, err
	}
	return agg.finalize()
}

// ===== SSE parser =====

type sseEvent struct {
	name string
	data []byte
}

type sseAggregator interface {
	feed(evt sseEvent)
	finalize() ([]byte, error)
}

// feedSSE 按 W3C EventSource 规则解析行：空行分隔事件，data: 行可多次出现并拼接，
// 忽略以 ":" 起头的注释行。SSE 协议没有"事件结束"标记，只有空行分割。
func feedSSE(raw []byte, agg sseAggregator) error {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	// SSE 中单事件 data 可能比默认 64 KB 还大（如完整的 message_start payload），放宽到 1 MB。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var name string
	var data strings.Builder

	flush := func() {
		if data.Len() == 0 && name == "" {
			return
		}
		payload := []byte(data.String())
		evt := sseEvent{name: name, data: payload}
		name = ""
		data.Reset()
		if len(payload) > 0 {
			agg.feed(evt)
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			name = strings.TrimSpace(line[len("event:"):])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			d := line[len("data:"):]
			if len(d) > 0 && d[0] == ' ' {
				d = d[1:]
			}
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(d)
			continue
		}
		// id:/retry: 等其它字段调试日志不需要
	}
	flush()
	return scanner.Err()
}

// ===== Anthropic aggregator =====
//
// 处理 message_start / content_block_start / content_block_delta /
// message_delta 等事件，重建 Anthropic Messages API 的最终 Message 结构。
// 解析失败的单个事件会被静默跳过（部分截断的流也能尽力还原已有内容）。

type anthropicAggregator struct {
	message map[string]any
	blocks  []any
}

func newAnthropicAggregator() *anthropicAggregator {
	return &anthropicAggregator{}
}

func (a *anthropicAggregator) feed(evt sseEvent) {
	if bytes.Equal(bytes.TrimSpace(evt.data), []byte("[DONE]")) {
		return
	}
	var obj map[string]any
	if err := sonic.Unmarshal(evt.data, &obj); err != nil {
		return
	}
	t, _ := obj["type"].(string)
	if t == "" {
		t = evt.name
	}
	switch t {
	case "message_start":
		if m, ok := obj["message"].(map[string]any); ok {
			a.message = m
			if cb, ok := m["content"].([]any); ok {
				a.blocks = cb
			} else {
				a.blocks = []any{}
			}
		}
	case "content_block_start":
		idx, ok := indexOfAny(obj["index"])
		if !ok {
			return
		}
		if cb, ok := obj["content_block"].(map[string]any); ok {
			a.ensureBlock(idx)
			a.blocks[idx] = cloneMap(cb)
		}
	case "content_block_delta":
		idx, ok := indexOfAny(obj["index"])
		if !ok {
			return
		}
		delta, _ := obj["delta"].(map[string]any)
		if delta == nil {
			return
		}
		a.ensureBlock(idx)
		block, _ := a.blocks[idx].(map[string]any)
		if block == nil {
			block = map[string]any{}
			a.blocks[idx] = block
		}
		dt, _ := delta["type"].(string)
		switch dt {
		case "text_delta":
			s, _ := delta["text"].(string)
			cur, _ := block["text"].(string)
			block["text"] = cur + s
		case "input_json_delta":
			s, _ := delta["partial_json"].(string)
			cur, _ := block["partial_json"].(string)
			block["partial_json"] = cur + s
		case "thinking_delta":
			s, _ := delta["thinking"].(string)
			cur, _ := block["thinking"].(string)
			block["thinking"] = cur + s
		case "signature_delta":
			s, _ := delta["signature"].(string)
			cur, _ := block["signature"].(string)
			block["signature"] = cur + s
		}
	case "message_delta":
		if a.message == nil {
			return
		}
		if delta, ok := obj["delta"].(map[string]any); ok {
			for k, v := range delta {
				a.message[k] = v
			}
		}
		if usage, ok := obj["usage"].(map[string]any); ok {
			a.message["usage"] = usage
		}
	}
}

func (a *anthropicAggregator) ensureBlock(idx int) {
	for len(a.blocks) <= idx {
		a.blocks = append(a.blocks, map[string]any{})
	}
}

func (a *anthropicAggregator) finalize() ([]byte, error) {
	if a.message == nil {
		return nil, errors.New("anthropic sse: no message_start event")
	}
	a.message["content"] = a.blocks
	return sonic.Marshal(a.message)
}

// ===== OpenAI aggregator =====
//
// 处理 chat.completion.chunk，按 choices[].delta 累积成最终的 chat.completion 结构。
// content / reasoning_content / tool_calls.function.arguments 都是 string-append 语义；
// 其余 delta 字段以最后一次出现的为准。

type openAIAggregator struct {
	completion map[string]any
	choices    map[int]map[string]any
}

func newOpenAIAggregator() *openAIAggregator {
	return &openAIAggregator{choices: map[int]map[string]any{}}
}

func (a *openAIAggregator) feed(evt sseEvent) {
	if bytes.Equal(bytes.TrimSpace(evt.data), []byte("[DONE]")) {
		return
	}
	var chunk map[string]any
	if err := sonic.Unmarshal(evt.data, &chunk); err != nil {
		return
	}
	if a.completion == nil {
		a.completion = map[string]any{
			"object": "chat.completion",
		}
		for _, k := range []string{"id", "created", "model", "system_fingerprint"} {
			if v, ok := chunk[k]; ok {
				a.completion[k] = v
			}
		}
	}
	if rcs, ok := chunk["choices"].([]any); ok {
		for _, rc := range rcs {
			c, ok := rc.(map[string]any)
			if !ok {
				continue
			}
			idx, _ := indexOfAny(c["index"])
			existing := a.choices[idx]
			if existing == nil {
				existing = map[string]any{"index": idx, "message": map[string]any{}}
				a.choices[idx] = existing
			}
			msg, _ := existing["message"].(map[string]any)
			if delta, ok := c["delta"].(map[string]any); ok {
				for k, v := range delta {
					switch k {
					case "content":
						if s, ok := v.(string); ok {
							cur, _ := msg["content"].(string)
							msg["content"] = cur + s
						}
					case "reasoning_content":
						if s, ok := v.(string); ok {
							cur, _ := msg["reasoning_content"].(string)
							msg["reasoning_content"] = cur + s
						}
					case "role":
						msg["role"] = v
					case "tool_calls":
						if newCalls, ok := v.([]any); ok {
							cur, _ := msg["tool_calls"].([]any)
							msg["tool_calls"] = mergeOpenAIToolCalls(cur, newCalls)
						}
					default:
						msg[k] = v
					}
				}
			}
			if fr, ok := c["finish_reason"]; ok && fr != nil {
				existing["finish_reason"] = fr
			}
		}
	}
	if usage, ok := chunk["usage"].(map[string]any); ok {
		a.completion["usage"] = usage
	}
}

func (a *openAIAggregator) finalize() ([]byte, error) {
	if a.completion == nil {
		return nil, errors.New("openai sse: no chunks received")
	}
	maxIdx := -1
	for idx := range a.choices {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	out := make([]any, 0, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		if c, ok := a.choices[i]; ok {
			out = append(out, c)
		}
	}
	a.completion["choices"] = out
	return sonic.Marshal(a.completion)
}

// ===== helpers =====

func indexOfAny(v any) (int, bool) {
	if f, ok := v.(float64); ok {
		return int(f), true
	}
	return 0, false
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// mergeOpenAIToolCalls 处理 OpenAI tool_calls 的增量合并。
// 每个 delta tool_call 形如 {index, id?, type?, function?:{name?, arguments?}}，
// function.arguments 是 string-append（增量 JSON 片段），其余字段后写覆盖。
func mergeOpenAIToolCalls(existing, deltas []any) []any {
	result := existing
	for _, d := range deltas {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}
		idx, _ := indexOfAny(dm["index"])
		for len(result) <= idx {
			result = append(result, map[string]any{})
		}
		cur, _ := result[idx].(map[string]any)
		if cur == nil {
			cur = map[string]any{}
			result[idx] = cur
		}
		for k, v := range dm {
			if k == "index" {
				continue
			}
			if k == "function" {
				newFn, _ := v.(map[string]any)
				if newFn == nil {
					continue
				}
				curFn, _ := cur["function"].(map[string]any)
				if curFn == nil {
					curFn = map[string]any{}
					cur["function"] = curFn
				}
				for fk, fv := range newFn {
					if fk == "arguments" {
						if s, ok := fv.(string); ok {
							ex, _ := curFn["arguments"].(string)
							curFn["arguments"] = ex + s
						}
					} else {
						curFn[fk] = fv
					}
				}
				continue
			}
			cur[k] = v
		}
	}
	return result
}
