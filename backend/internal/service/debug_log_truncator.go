package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// 字段级截断默认阈值。所有阈值都按 UTF-8 字节算。
// 调试日志的本质是定位问题的辅助手段,而不是完整复刻客户对话,
// 所以包括 system 在内的所有文本字段统一一个较小的上限。
const (
	// 单条文本字段(system / message.content / content[].text / tool_result.content /
	// thinking / instructions 等)的软上限
	debugLogFieldTextLimit = 1024
	// 工具调用 input / arguments 的软上限
	debugLogFieldToolLimit = 1024
	// 头尾各保留多少字节(头+尾合计 = limit,中间替换为占位)
	debugLogHeadTailSide = 512
	// 最近 N 条 message 的 tool_result 走正常字段级截断;更早的整段替换为占位。
	// 历史 tool_result 内容对当前轮的调试价值随时间快速衰减,只留 tool_use_id 引用足够。
	debugLogRecentMessagesKept = 2
)

// debugLogSide 标识截断目标(请求或响应),用于把字段截断记录归类到 TruncationInfo 的不同槽位。
type debugLogSide string

const (
	debugLogSideRequest  debugLogSide = "request"
	debugLogSideResponse debugLogSide = "response"
)

// FieldCut 描述单个字段被截断的元信息
type FieldCut struct {
	Path     string `json:"path"`           // 字段路径,如 "messages[3].content[0].text"
	Original int    `json:"original_bytes"` // 原始字节数
	Cut      int    `json:"cut_bytes"`      // 被砍掉的字节数
	Mode     string `json:"mode"`           // "tail" 或 "head_tail"
}

// TruncationInfo 一次截断的完整元信息,序列化后写入 truncation_info 列
type TruncationInfo struct {
	Request              []FieldCut `json:"request,omitempty"`
	Response             []FieldCut `json:"response,omitempty"`
	ImagesStripped       int        `json:"images_stripped,omitempty"`
	ToolResultsCut       int        `json:"tool_results_cut,omitempty"`
	ToolResultsElided    int        `json:"tool_results_elided,omitempty"`  // 历史 tool_result 被替换为占位的数量
	ToolsSimplified      int        `json:"tools_simplified,omitempty"`     // tools 数组被简化为名字列表的工具数
	ToolsBytesSaved      int        `json:"tools_bytes_saved,omitempty"`    // tools 简化节省的原始字节数(估算)
	SignaturesStripped   int        `json:"signatures_stripped,omitempty"`  // 删除的 thinking signature / redacted thinking 块数
	CacheControlStripped int        `json:"cache_control_stripped,omitempty"` // 删除的 cache_control 标记数
	OverallCutBytes      int        `json:"overall_cut_bytes,omitempty"`    // 字段级截断后仍超硬上限,被字节硬截
	AggregationFailed    bool       `json:"aggregation_failed,omitempty"`   // 流式聚合失败,走原始字节兜底
	SmartFailed          bool       `json:"smart_failed,omitempty"`         // JSON 解析失败,走原始字节兜底
}

func (t *TruncationInfo) hasAny() bool {
	if t == nil {
		return false
	}
	return len(t.Request) > 0 || len(t.Response) > 0 ||
		t.ImagesStripped > 0 || t.ToolResultsCut > 0 || t.ToolResultsElided > 0 ||
		t.ToolsSimplified > 0 || t.SignaturesStripped > 0 || t.CacheControlStripped > 0 ||
		t.OverallCutBytes > 0 ||
		t.AggregationFailed || t.SmartFailed
}

// truncCtx 携带遍历过程中需要累加的统计量,避免在多层递归里传递多个指针参数。
type truncCtx struct {
	cuts                 []FieldCut
	stripped             int
	toolResults          int
	toolResultsElided    int
	toolsSimplified      int
	toolsBytesSaved      int
	signaturesStripped   int
	cacheControlStripped int
}

// SmartTruncate 解析 JSON body,按协议做字段级截断,返回新的 JSON 字节 + 截断记录。
//
// 如果 body 不是合法 JSON,返回 ok=false,调用方走原始字节兜底。
func SmartTruncate(body []byte, protocol DebugLogProtocol, side debugLogSide, info *TruncationInfo) (out []byte, ok bool) {
	if len(body) == 0 {
		return body, true
	}
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return body, false
	}

	tc := &truncCtx{}

	switch protocol {
	case DebugLogProtocolAnthropic:
		if side == debugLogSideRequest {
			truncateAnthropicRequest(root, tc)
		} else {
			truncateAnthropicResponse(root, tc)
		}
	case DebugLogProtocolOpenAI:
		if side == debugLogSideRequest {
			truncateOpenAIRequest(root, tc)
		} else {
			truncateOpenAIResponse(root, tc)
		}
	default:
		return body, false
	}

	if info != nil {
		if side == debugLogSideRequest {
			info.Request = append(info.Request, tc.cuts...)
		} else {
			info.Response = append(info.Response, tc.cuts...)
		}
		info.ImagesStripped += tc.stripped
		info.ToolResultsCut += tc.toolResults
		info.ToolResultsElided += tc.toolResultsElided
		info.ToolsSimplified += tc.toolsSimplified
		info.ToolsBytesSaved += tc.toolsBytesSaved
		info.SignaturesStripped += tc.signaturesStripped
		info.CacheControlStripped += tc.cacheControlStripped
	}

	out, err := json.Marshal(root)
	if err != nil {
		return body, false
	}
	return out, true
}

// truncateText 对一个字符串字段做截断,返回新字符串。
// 截断策略:统一头 + 尾截法,各保留 debugLogHeadTailSide 字节。
// 若 limit < 2*side,退化为只截尾(典型场景下不会发生,仅做防御)。
func truncateText(s string, limit int, path string, tc *truncCtx) string {
	if len(s) <= limit {
		return s
	}
	original := len(s)
	side := debugLogHeadTailSide
	if limit < 2*side {
		side = limit / 2
		if side <= 0 {
			cut := safeCut(s, limit)
			truncated := s[:cut] + fmt.Sprintf("...[truncated %d bytes]", original-cut)
			tc.cuts = append(tc.cuts, FieldCut{
				Path: path, Original: original, Cut: original - len(truncated), Mode: "tail",
			})
			return truncated
		}
	}
	head := safeCut(s, side)
	tailStart := safeStart(s, len(s)-side)
	mid := tailStart - head
	truncated := s[:head] + fmt.Sprintf("\n...[truncated %d bytes]...\n", mid) + s[tailStart:]
	tc.cuts = append(tc.cuts, FieldCut{
		Path:     path,
		Original: original,
		Cut:      original - len(truncated),
		Mode:     "head_tail",
	})
	return truncated
}

// safeCut 在不超过 limit 的前提下,返回 UTF-8 安全的字节切割位置。
func safeCut(s string, limit int) int {
	if limit >= len(s) {
		return len(s)
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return cut
}

// safeStart 给定一个起点,向右调整到 UTF-8 字符边界。
func safeStart(s string, start int) int {
	if start <= 0 {
		return 0
	}
	if start >= len(s) {
		return len(s)
	}
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return start
}

// ===== Anthropic =====

// truncateAnthropicRequest 处理 /v1/messages 请求体。
// 顶层结构:{ system, messages: [{role, content}], tools, ... }
func truncateAnthropicRequest(root any, tc *truncCtx) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}

	// system: 可能是 string 或 [{type:"text", text}]
	if sys, exists := m["system"]; exists {
		m["system"] = truncateAnthropicSystem(sys, tc)
	}

	// tools: 简化为 name 字符串数组,description / input_schema 全砍
	if tools, ok := m["tools"].([]any); ok && len(tools) > 0 {
		m["tools"] = simplifyToolsToNames(tools, anthropicToolName, tc)
	}

	// messages
	if msgs, ok := m["messages"].([]any); ok {
		total := len(msgs)
		for i, msg := range msgs {
			msgMap, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			// 距离末尾 < debugLogRecentMessagesKept 的是"最近"消息,其它视为"历史"
			isHistorical := total-i > debugLogRecentMessagesKept
			truncateAnthropicMessageContent(msgMap, fmt.Sprintf("messages[%d]", i), tc, isHistorical)
		}
	}
}

// stripCacheControl 删除 message / content block / system 块上的 cache_control 标记。
// 调试日志无需关心客户端的缓存策略。
func stripCacheControl(m map[string]any, tc *truncCtx) {
	if _, ok := m["cache_control"]; ok {
		delete(m, "cache_control")
		tc.cacheControlStripped++
	}
}

// simplifyToolsToNames 把 tools 数组压缩成 ["name1", "name2", ...] 字符串数组。
// 调试日志只需要知道当时哪些工具可用,description/schema 几乎用不上但占用大头。
// extract 用于按协议从单个 tool 对象里取名字(Anthropic 是 tool.name,
// OpenAI chat.completions 是 tool.function.name)。
func simplifyToolsToNames(tools []any, extract func(any) string, tc *truncCtx) []any {
	original, _ := json.Marshal(tools)
	names := make([]any, 0, len(tools))
	for _, t := range tools {
		name := extract(t)
		if name == "" {
			name = "<unnamed>"
		}
		names = append(names, name)
	}
	simplified, _ := json.Marshal(names)
	tc.toolsSimplified += len(tools)
	if saved := len(original) - len(simplified); saved > 0 {
		tc.toolsBytesSaved += saved
	}
	return names
}

func anthropicToolName(t any) string {
	m, ok := t.(map[string]any)
	if !ok {
		return ""
	}
	name, _ := m["name"].(string)
	return name
}

func openAIToolName(t any) string {
	m, ok := t.(map[string]any)
	if !ok {
		return ""
	}
	// chat.completions: {type:"function", function:{name, parameters, ...}}
	if fn, ok := m["function"].(map[string]any); ok {
		if name, ok := fn["name"].(string); ok && name != "" {
			return name
		}
	}
	// Responses API: 顶层直接是 {type:"function"|"web_search"|..., name?}
	if name, ok := m["name"].(string); ok && name != "" {
		return name
	}
	// 内置工具(web_search / code_interpreter 等)无 name,用 type 兜底
	if typ, ok := m["type"].(string); ok && typ != "" {
		return typ
	}
	return ""
}

// truncateAnthropicResponse 处理 /v1/messages 响应体(已聚合的最终 message)。
// 顶层结构:{ role, content: [blocks], usage, ... }
func truncateAnthropicResponse(root any, tc *truncCtx) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	if content, ok := m["content"].([]any); ok {
		truncateAnthropicBlocks(content, "content", tc, false)
	}
}

func truncateAnthropicSystem(sys any, tc *truncCtx) any {
	switch v := sys.(type) {
	case string:
		return truncateText(v, debugLogFieldTextLimit, "system", tc)
	case []any:
		for i, item := range v {
			if blk, ok := item.(map[string]any); ok {
				stripCacheControl(blk, tc)
				if txt, ok := blk["text"].(string); ok {
					blk["text"] = truncateText(txt, debugLogFieldTextLimit, fmt.Sprintf("system[%d].text", i), tc)
				}
			}
		}
	}
	return sys
}

func truncateAnthropicMessageContent(msg map[string]any, prefix string, tc *truncCtx, isHistorical bool) {
	stripCacheControl(msg, tc)
	switch v := msg["content"].(type) {
	case string:
		msg["content"] = truncateText(v, debugLogFieldTextLimit, prefix+".content", tc)
	case []any:
		truncateAnthropicBlocks(v, prefix+".content", tc, isHistorical)
	}
}

func truncateAnthropicBlocks(blocks []any, prefix string, tc *truncCtx, isHistorical bool) {
	for i, item := range blocks {
		blk, ok := item.(map[string]any)
		if !ok {
			continue
		}
		stripCacheControl(blk, tc)
		path := fmt.Sprintf("%s[%d]", prefix, i)
		t, _ := blk["type"].(string)
		switch t {
		case "text":
			if txt, ok := blk["text"].(string); ok {
				blk["text"] = truncateText(txt, debugLogFieldTextLimit, path+".text", tc)
			}
		case "thinking":
			if txt, ok := blk["thinking"].(string); ok {
				blk["thinking"] = truncateText(txt, debugLogFieldTextLimit, path+".thinking", tc)
			}
			// signature 对调试无价值(HMAC 防篡改用),整段删除
			if _, ok := blk["signature"]; ok {
				delete(blk, "signature")
				tc.signaturesStripped++
			}
		case "redacted_thinking":
			// 整个块是加密 token,内部 data 字段对调试无意义,删数据保结构
			if _, ok := blk["data"]; ok {
				delete(blk, "data")
				tc.signaturesStripped++
			}
		case "image":
			if src, ok := blk["source"].(map[string]any); ok {
				if data, ok := src["data"].(string); ok && len(data) > 0 {
					mt, _ := src["media_type"].(string)
					src["data"] = fmt.Sprintf("<image base64 stripped: media_type=%s, %d bytes>", mt, len(data))
					tc.stripped++
				}
			}
		case "tool_use":
			truncateAnthropicToolUseInput(blk, path, tc)
		case "tool_result":
			truncateAnthropicToolResult(blk, path, tc, isHistorical)
		}
	}
}

// truncateAnthropicToolUseInput 处理 tool_use 块的 input/partial_json 字段。
// input 是对象,保持结构,递归截内部 string;partial_json 字符串先尝试 parse 回对象。
func truncateAnthropicToolUseInput(blk map[string]any, path string, tc *truncCtx) {
	// 流式聚合产物:partial_json 字符串。能 parse 回对象就替换 input,失败保持字符串截断
	if pj, ok := blk["partial_json"].(string); ok {
		var parsed any
		if err := json.Unmarshal([]byte(pj), &parsed); err == nil {
			blk["input"] = parsed
			delete(blk, "partial_json")
		} else {
			blk["partial_json"] = truncateText(pj, debugLogFieldToolLimit, path+".partial_json", tc)
			return
		}
	}
	if input, ok := blk["input"]; ok && input != nil {
		blk["input"] = truncateJSONValueStrings(input, debugLogFieldToolLimit, path+".input", tc)
	}
}

// truncateJSONValueStrings 递归截断 JSON 值里所有 string 节点,保持对象/数组结构。
// 用于 tool_use.input 这类对象字段:既不破坏结构,又把长字符串内容压缩。
func truncateJSONValueStrings(v any, limit int, path string, tc *truncCtx) any {
	switch x := v.(type) {
	case string:
		if len(x) > limit {
			return truncateText(x, limit, path, tc)
		}
		return x
	case []any:
		for i := range x {
			x[i] = truncateJSONValueStrings(x[i], limit, fmt.Sprintf("%s[%d]", path, i), tc)
		}
		return x
	case map[string]any:
		for k, vv := range x {
			x[k] = truncateJSONValueStrings(vv, limit, path+"."+k, tc)
		}
		return x
	default:
		return v
	}
}

// truncateAnthropicToolResult 历史消息的 tool_result 整段替换为占位,只留 tool_use_id 引用;
// 最近 N 条消息走常规字段级截断。
func truncateAnthropicToolResult(blk map[string]any, path string, tc *truncCtx, isHistorical bool) {
	if isHistorical {
		toolUseID, _ := blk["tool_use_id"].(string)
		blk["content"] = fmt.Sprintf("<tool_result elided for historical tool_use_id=%s>", toolUseID)
		tc.toolResultsElided++
		return
	}
	switch content := blk["content"].(type) {
	case string:
		if len(content) > debugLogFieldTextLimit {
			blk["content"] = truncateText(content, debugLogFieldTextLimit, path+".content", tc)
			tc.toolResults++
		}
	case []any:
		truncatedAny := false
		for j, it := range content {
			sub, ok := it.(map[string]any)
			if !ok {
				continue
			}
			stripCacheControl(sub, tc)
			subPath := fmt.Sprintf("%s.content[%d]", path, j)
			st, _ := sub["type"].(string)
			switch st {
			case "text":
				if txt, ok := sub["text"].(string); ok && len(txt) > debugLogFieldTextLimit {
					sub["text"] = truncateText(txt, debugLogFieldTextLimit, subPath+".text", tc)
					truncatedAny = true
				}
			case "image":
				if src, ok := sub["source"].(map[string]any); ok {
					if data, ok := src["data"].(string); ok && len(data) > 0 {
						mt, _ := src["media_type"].(string)
						src["data"] = fmt.Sprintf("<image base64 stripped: media_type=%s, %d bytes>", mt, len(data))
						tc.stripped++
					}
				}
			}
		}
		if truncatedAny {
			tc.toolResults++
		}
	}
}

// ===== OpenAI =====

// truncateOpenAIRequest 处理 chat.completions / responses 两种请求体。
// chat.completions:{ messages: [{role, content, tool_calls}], ... }
// responses:{ input: ..., instructions, ... }
func truncateOpenAIRequest(root any, tc *truncCtx) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}

	// chat.completions 的 system 偶尔是顶层 system 字段
	if sys, ok := m["system"].(string); ok {
		m["system"] = truncateText(sys, debugLogFieldTextLimit, "system", tc)
	}
	if instr, ok := m["instructions"].(string); ok {
		m["instructions"] = truncateText(instr, debugLogFieldTextLimit, "instructions", tc)
	}

	// tools: 简化为 name 字符串数组
	if tools, ok := m["tools"].([]any); ok && len(tools) > 0 {
		m["tools"] = simplifyToolsToNames(tools, openAIToolName, tc)
	}

	if msgs, ok := m["messages"].([]any); ok {
		total := len(msgs)
		for i, msg := range msgs {
			msgMap, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			isHistorical := total-i > debugLogRecentMessagesKept
			truncateOpenAIMessage(msgMap, fmt.Sprintf("messages[%d]", i), tc, isHistorical)
		}
	}

	// Responses API: input 可能是 string 或数组
	if input, ok := m["input"]; ok {
		m["input"] = truncateOpenAIInput(input, "input", tc)
	}
}

// truncateOpenAIResponse 处理 chat.completion / response 响应体。
func truncateOpenAIResponse(root any, tc *truncCtx) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	// chat.completion: choices[].message
	if choices, ok := m["choices"].([]any); ok {
		for i, ch := range choices {
			chm, ok := ch.(map[string]any)
			if !ok {
				continue
			}
			if msg, ok := chm["message"].(map[string]any); ok {
				truncateOpenAIMessage(msg, fmt.Sprintf("choices[%d].message", i), tc, false)
			}
			if delta, ok := chm["delta"].(map[string]any); ok {
				truncateOpenAIMessage(delta, fmt.Sprintf("choices[%d].delta", i), tc, false)
			}
		}
	}
	// Responses API: output[].content[].text
	if output, ok := m["output"].([]any); ok {
		for i, it := range output {
			itm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			path := fmt.Sprintf("output[%d]", i)
			if content, ok := itm["content"].([]any); ok {
				for j, c := range content {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					if txt, ok := cm["text"].(string); ok {
						cm["text"] = truncateText(txt, debugLogFieldTextLimit, fmt.Sprintf("%s.content[%d].text", path, j), tc)
					}
				}
			}
		}
	}
}

func truncateOpenAIMessage(msg map[string]any, prefix string, tc *truncCtx, isHistorical bool) {
	stripCacheControl(msg, tc)
	// role="tool" 的历史消息整段替换为占位,只留 tool_call_id 引用
	if isHistorical {
		if role, _ := msg["role"].(string); role == "tool" {
			toolCallID, _ := msg["tool_call_id"].(string)
			msg["content"] = fmt.Sprintf("<tool result elided for historical tool_call_id=%s>", toolCallID)
			tc.toolResultsElided++
			return
		}
	}
	// content: string 或 [{type:"text",text} | {type:"image_url",image_url}]
	switch v := msg["content"].(type) {
	case string:
		msg["content"] = truncateText(v, debugLogFieldTextLimit, prefix+".content", tc)
	case []any:
		for i, item := range v {
			sub, ok := item.(map[string]any)
			if !ok {
				continue
			}
			subPath := fmt.Sprintf("%s.content[%d]", prefix, i)
			t, _ := sub["type"].(string)
			switch t {
			case "text", "input_text", "output_text":
				if txt, ok := sub["text"].(string); ok {
					sub["text"] = truncateText(txt, debugLogFieldTextLimit, subPath+".text", tc)
				}
			case "image_url":
				// data URI 才裁,http(s) URL 保留
				if iu, ok := sub["image_url"].(map[string]any); ok {
					if url, ok := iu["url"].(string); ok && strings.HasPrefix(url, "data:") && len(url) > 256 {
						iu["url"] = fmt.Sprintf("<data-uri stripped: %d bytes>", len(url))
						tc.stripped++
					}
				}
			case "input_image":
				if img, ok := sub["image_url"].(string); ok && strings.HasPrefix(img, "data:") && len(img) > 256 {
					sub["image_url"] = fmt.Sprintf("<data-uri stripped: %d bytes>", len(img))
					tc.stripped++
				}
			}
		}
	}

	// reasoning_content (DeepSeek/o1 风格)
	if rc, ok := msg["reasoning_content"].(string); ok {
		msg["reasoning_content"] = truncateText(rc, debugLogFieldTextLimit, prefix+".reasoning_content", tc)
	}

	// tool_calls[].function.arguments 是 JSON 字符串
	if tcs, ok := msg["tool_calls"].([]any); ok {
		for i, item := range tcs {
			tcm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if fn, ok := tcm["function"].(map[string]any); ok {
				if args, ok := fn["arguments"].(string); ok && len(args) > debugLogFieldToolLimit {
					fn["arguments"] = truncateText(args, debugLogFieldToolLimit, fmt.Sprintf("%s.tool_calls[%d].function.arguments", prefix, i), tc)
				}
			}
		}
	}
}

func truncateOpenAIInput(input any, prefix string, tc *truncCtx) any {
	switch v := input.(type) {
	case string:
		return truncateText(v, debugLogFieldTextLimit, prefix, tc)
	case []any:
		for i, item := range v {
			sub, ok := item.(map[string]any)
			if !ok {
				continue
			}
			truncateOpenAIMessage(sub, fmt.Sprintf("%s[%d]", prefix, i), tc, false)
		}
	}
	return input
}
