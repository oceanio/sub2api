package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSmartTruncate_NonJSON(t *testing.T) {
	info := &TruncationInfo{}
	out, ok := SmartTruncate([]byte("not json at all"), DebugLogProtocolAnthropic, "request", info)
	if ok {
		t.Fatalf("expected ok=false for non-JSON input, got ok=true (out=%s)", out)
	}
}

func TestSmartTruncate_AnthropicRequest_LongText(t *testing.T) {
	long := strings.Repeat("a", 4096) // > 1KB,应触发 head_tail 模式
	req := map[string]any{
		"model": "claude-opus-4-7",
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": long,
			},
		},
	}
	body, _ := json.Marshal(req)

	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, "request", info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if len(out) >= len(body) {
		t.Fatalf("expected out smaller than input, got %d >= %d", len(out), len(body))
	}
	if len(info.Request) != 1 {
		t.Fatalf("expected 1 field cut, got %d", len(info.Request))
	}
	if info.Request[0].Path != "messages[0].content" {
		t.Errorf("wrong path: %s", info.Request[0].Path)
	}
	if info.Request[0].Mode != "head_tail" {
		t.Errorf("expected head_tail mode, got %s", info.Request[0].Mode)
	}
	// 验证 JSON 仍合法
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	// 验证 truncate 标记存在
	msgs := parsed["messages"].([]any)
	c := msgs[0].(map[string]any)["content"].(string)
	if !strings.Contains(c, "truncated") {
		t.Errorf("expected truncation marker in content, got prefix: %s", c[:50])
	}
}

func TestSmartTruncate_BelowThreshold_NotCut(t *testing.T) {
	// 文本字段限值 debugLogFieldTextLimit=256，<= 256 不应被截。
	body, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": strings.Repeat("x", debugLogFieldTextLimit)},
		},
	})
	info := &TruncationInfo{}
	_, ok := SmartTruncate(body, DebugLogProtocolAnthropic, "request", info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if len(info.Request) != 0 {
		t.Errorf("expected no cut at exactly threshold, got %d cuts", len(info.Request))
	}
}

func TestSmartTruncate_AnthropicImage_Stripped(t *testing.T) {
	bigData := strings.Repeat("A", 1024) // 模拟 base64
	req := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/jpeg",
							"data":       bigData,
						},
					},
					map[string]any{
						"type": "text",
						"text": "describe this",
					},
				},
			},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, "request", info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if info.ImagesStripped != 1 {
		t.Errorf("expected ImagesStripped=1, got %d", info.ImagesStripped)
	}
	if strings.Contains(string(out), bigData) {
		t.Errorf("base64 data should have been stripped")
	}
	if !strings.Contains(string(out), "image base64 stripped") {
		t.Errorf("placeholder missing")
	}
}

func TestSmartTruncate_AnthropicResponse_AggregatedJSON(t *testing.T) {
	long := strings.Repeat("x", 4096) // > 1KB,统一头尾截
	resp := map[string]any{
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "text", "text": long},
		},
	}
	body, _ := json.Marshal(resp)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, "response", info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if len(info.Response) != 1 {
		t.Fatalf("expected 1 cut, got %d", len(info.Response))
	}
	if info.Response[0].Mode != "head_tail" {
		t.Errorf("expected head_tail mode, got %s", info.Response[0].Mode)
	}
	if !strings.Contains(string(out), "[truncated") {
		t.Errorf("expected truncation marker")
	}
}

func TestSmartTruncate_OpenAI_ToolCallArgs(t *testing.T) {
	bigArgs := strings.Repeat("z", 8*1024)
	resp := map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []any{
						map[string]any{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "search",
								"arguments": bigArgs,
							},
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(resp)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolOpenAI, "response", info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if len(info.Response) != 1 {
		t.Fatalf("expected 1 cut, got %d", len(info.Response))
	}
	if !strings.Contains(info.Response[0].Path, "tool_calls[0].function.arguments") {
		t.Errorf("wrong path: %s", info.Response[0].Path)
	}
	if strings.Contains(string(out), bigArgs) {
		t.Errorf("arguments should have been truncated")
	}
}

func TestSmartTruncate_AnthropicTools_SimplifiedToNames(t *testing.T) {
	bigDesc := strings.Repeat("d", 2048)
	req := map[string]any{
		"model": "claude-opus-4-7",
		"tools": []any{
			map[string]any{
				"name":         "Bash",
				"description":  bigDesc,
				"input_schema": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}},
			},
			map[string]any{
				"name":         "Read",
				"description":  bigDesc,
				"input_schema": map[string]any{"type": "object"},
			},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, debugLogSideRequest, info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if info.ToolsSimplified != 2 {
		t.Errorf("expected ToolsSimplified=2, got %d", info.ToolsSimplified)
	}
	if info.ToolsBytesSaved <= 0 {
		t.Errorf("expected positive ToolsBytesSaved, got %d", info.ToolsBytesSaved)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	tools, _ := parsed["tools"].([]any)
	if len(tools) != 2 || tools[0] != "Bash" || tools[1] != "Read" {
		t.Errorf("expected tools=[Bash,Read], got %v", tools)
	}
	if len(out) >= len(body)/4 {
		t.Errorf("expected significant size reduction, got out=%d body=%d", len(out), len(body))
	}
}

func TestSmartTruncate_OpenAITools_SimplifiedToNames(t *testing.T) {
	bigDesc := strings.Repeat("d", 1024)
	req := map[string]any{
		"model": "gpt-5",
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "search",
					"description": bigDesc,
					"parameters":  map[string]any{"type": "object"},
				},
			},
			// Responses API 风格
			map[string]any{"type": "web_search"},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolOpenAI, debugLogSideRequest, info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if info.ToolsSimplified != 2 {
		t.Errorf("expected ToolsSimplified=2, got %d", info.ToolsSimplified)
	}
	var parsed map[string]any
	_ = json.Unmarshal(out, &parsed)
	tools, _ := parsed["tools"].([]any)
	if len(tools) != 2 || tools[0] != "search" || tools[1] != "web_search" {
		t.Errorf("expected [search, web_search], got %v", tools)
	}
}

func TestSmartTruncate_AnthropicThinking_SignatureStripped(t *testing.T) {
	// thinking signature 通常出现在 assistant 响应中。请求侧 messages 里若含
	// assistant thinking 块，只有在"最后一个 user 之后"才会进入字段级处理。
	// 这里把 thinking assistant 放在 user 之后（典型场景：续接 assistant 上一轮的
	// thinking + tool_use，然后再发 tool_result 回去 → 但 tool_result 是 user 角色
	// 在更后面）。为了让 thinking 块进入"保留区"，需放在最后一个 user 后面。
	req := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":      "thinking",
						"thinking":  "let me think...",
						"signature": strings.Repeat("s", 2048),
					},
					map[string]any{
						"type": "redacted_thinking",
						"data": strings.Repeat("e", 4096),
					},
				},
			},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, debugLogSideRequest, info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if info.SignaturesStripped != 2 {
		t.Errorf("expected SignaturesStripped=2, got %d", info.SignaturesStripped)
	}
	if strings.Contains(string(out), strings.Repeat("s", 100)) {
		t.Errorf("signature should be removed")
	}
	if strings.Contains(string(out), strings.Repeat("e", 100)) {
		t.Errorf("redacted_thinking data should be removed")
	}
}

func TestSmartTruncate_AnthropicHistoricalMessages_Elided(t *testing.T) {
	// 历史轮次（含 tool_result 大输出）应被整段聚合，只保留最后一个 user 起的部分。
	bigOutput := strings.Repeat("x", 4096)
	req := map[string]any{
		"messages": []any{
			// 历史 4 条
			map[string]any{"role": "user", "content": "first"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "call_1", "name": "Bash", "input": map[string]any{"cmd": "ls"}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": bigOutput},
			}},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "call_2", "name": "Bash", "input": map[string]any{"cmd": "pwd"}},
			}},
			// 最后一个 user：从这里开始保留
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "call_2", "content": "/tmp"},
			}},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, debugLogSideRequest, info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if info.HistoricalMessagesElided != 4 {
		t.Errorf("expected HistoricalMessagesElided=4, got %d", info.HistoricalMessagesElided)
	}
	// 历史的 4KB 输出消失,最近的 /tmp 还在
	if strings.Contains(string(out), strings.Repeat("x", 100)) {
		t.Errorf("historical tool_result content should be elided")
	}
	if !strings.Contains(string(out), "/tmp") {
		t.Errorf("recent tool_result should be preserved")
	}
	// 占位消息应描述历史 block 类型分布（tool_use/tool_result 计数）
	if !strings.Contains(string(out), "historical 4 message(s) elided") {
		t.Errorf("expected aggregate placeholder, got: %s", string(out))
	}
}

func TestSmartTruncate_AnthropicCacheControl_Stripped(t *testing.T) {
	req := map[string]any{
		"system": []any{
			map[string]any{"type": "text", "text": "you are helpful", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"messages": []any{
			map[string]any{
				"role":          "user",
				"cache_control": map[string]any{"type": "ephemeral"},
				"content": []any{
					map[string]any{"type": "text", "text": "hi", "cache_control": map[string]any{"type": "ephemeral"}},
				},
			},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, debugLogSideRequest, info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if info.CacheControlStripped != 3 {
		t.Errorf("expected CacheControlStripped=3, got %d", info.CacheControlStripped)
	}
	if strings.Contains(string(out), "cache_control") {
		t.Errorf("cache_control should be removed: %s", out)
	}
}

func TestSmartTruncate_AnthropicToolUseInput_StructPreserved(t *testing.T) {
	bigVal := strings.Repeat("y", 4096)
	req := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "go"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{
					"type": "tool_use",
					"id":   "call_x",
					"name": "Bash",
					"input": map[string]any{
						"command": bigVal,
						"timeout": 5000,
					},
				},
			}},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, debugLogSideRequest, info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	msgs := parsed["messages"].([]any)
	tu := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	input, ok := tu["input"].(map[string]any)
	if !ok {
		t.Fatalf("input must remain an object, got %T", tu["input"])
	}
	if _, ok := input["timeout"]; !ok {
		t.Errorf("non-string field timeout should be preserved")
	}
	cmd, _ := input["command"].(string)
	if !strings.Contains(cmd, "truncated") {
		t.Errorf("long string field should be truncated, got: %.80s", cmd)
	}
	if _, ok := tu["input_truncated"]; ok {
		t.Errorf("legacy input_truncated field should not be created anymore")
	}
}

func TestSmartTruncate_PreservesShortFields(t *testing.T) {
	req := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": "hello"},
		},
	}
	body, _ := json.Marshal(req)
	info := &TruncationInfo{}
	out, ok := SmartTruncate(body, DebugLogProtocolAnthropic, "request", info)
	if !ok {
		t.Fatalf("SmartTruncate failed")
	}
	if len(info.Request) != 0 {
		t.Errorf("expected no cuts for short fields, got %d", len(info.Request))
	}
	if string(out) != string(body) {
		// 由于 map 序列化顺序可能不同,只比对结构等价
		var a, b map[string]any
		_ = json.Unmarshal(out, &a)
		_ = json.Unmarshal(body, &b)
		oa, _ := json.Marshal(a)
		ob, _ := json.Marshal(b)
		if string(oa) != string(ob) {
			t.Errorf("structure changed unexpectedly")
		}
	}
}
