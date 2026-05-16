package service

import (
	"encoding/json" // 仅用于 json.RawMessage 类型；sonic 兼容该类型
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	// sonic 是 bytedance JIT 加速的 JSON 库，drop-in 替代 encoding/json
	// 的 Marshal/Unmarshal。仅在 debug-log 路径使用，主链路（usage 解析等）
	// 仍用 encoding/json。debug-log 对 JSON 没有强语义要求（本来就允许有损
	// 截断），适合放宽。
	sonic "github.com/bytedance/sonic"
)

// 字段级截断默认阈值。所有阈值都按 UTF-8 字节算。
// 调试日志的本质是定位问题的辅助手段,而不是完整复刻客户对话,
// 所以包括 system 在内的所有文本字段统一一个较小的上限。
const (
	// 单条文本字段(system / message.content / content[].text / tool_result.content /
	// thinking / instructions 等)的软上限
	debugLogFieldTextLimit = 256
	// 工具调用 input / arguments 的软上限
	debugLogFieldToolLimit = 256
	// 头尾各保留多少字节(头+尾合计 = limit,中间替换为占位)
	debugLogHeadTailSide = 128
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
	Request                  []FieldCut `json:"request,omitempty"`
	Response                 []FieldCut `json:"response,omitempty"`
	ImagesStripped           int        `json:"images_stripped,omitempty"`
	ToolResultsCut           int        `json:"tool_results_cut,omitempty"`
	ToolResultsElided        int        `json:"tool_results_elided,omitempty"`        // 历史 tool_result 被替换为占位的数量
	HistoricalMessagesElided int        `json:"historical_messages_elided,omitempty"` // 历史轮次整段占位的消息数(content 被替换,role 保留)
	ToolsSimplified          int        `json:"tools_simplified,omitempty"`           // tools 数组被简化为名字列表的工具数
	ToolsBytesSaved          int        `json:"tools_bytes_saved,omitempty"`          // tools 简化节省的原始字节数(估算)
	SignaturesStripped       int        `json:"signatures_stripped,omitempty"`        // 删除的 thinking signature / redacted thinking 块数
	CacheControlStripped     int        `json:"cache_control_stripped,omitempty"`     // 删除的 cache_control 标记数
	OverallCutBytes          int        `json:"overall_cut_bytes,omitempty"`          // 字段级截断后仍超硬上限,被字节硬截
	AggregationFailed        bool       `json:"aggregation_failed,omitempty"`         // 流式聚合失败,走原始字节兜底
	SmartFailed              bool       `json:"smart_failed,omitempty"`               // JSON 解析失败,走原始字节兜底
}

func (t *TruncationInfo) hasAny() bool {
	if t == nil {
		return false
	}
	return len(t.Request) > 0 || len(t.Response) > 0 ||
		t.ImagesStripped > 0 || t.ToolResultsCut > 0 || t.ToolResultsElided > 0 ||
		t.HistoricalMessagesElided > 0 ||
		t.ToolsSimplified > 0 || t.SignaturesStripped > 0 || t.CacheControlStripped > 0 ||
		t.OverallCutBytes > 0 ||
		t.AggregationFailed || t.SmartFailed
}

// truncCtx 携带遍历过程中需要累加的统计量,避免在多层递归里传递多个指针参数。
type truncCtx struct {
	cuts                     []FieldCut
	stripped                 int
	toolResults              int
	toolResultsElided        int
	historicalMessagesElided int
	toolsSimplified          int
	toolsBytesSaved          int
	signaturesStripped       int
	cacheControlStripped     int
}

// SmartTruncate 解析 JSON body,按协议做字段级截断,返回新的 JSON 字节 + 截断记录。
//
// 如果 body 不是合法 JSON,返回 ok=false,调用方走原始字节兜底。
//
// 实现策略（D2 分层解析）：
//   - 请求侧：顶层用 map[string]json.RawMessage 浅层接收，避免把整个 body
//     爆成 map[string]any 树（1MB JSON 在该结构下膨胀 3-5x）。messages 数组
//     逐条 RawMessage：历史轮次只解析 role/content.type 做统计后整段丢弃；
//     最近一轮才完整 Unmarshal 到 map[string]any 改写。tools 数组只取 name。
//   - 响应侧：通常是单条 final message，结构简单，保持原 map[string]any 路径。
func SmartTruncate(body []byte, protocol DebugLogProtocol, side debugLogSide, info *TruncationInfo) (out []byte, ok bool) {
	if len(body) == 0 {
		return body, true
	}

	tc := &truncCtx{}

	// 请求侧走 D2 lazy 路径
	if side == debugLogSideRequest {
		switch protocol {
		case DebugLogProtocolAnthropic, DebugLogProtocolOpenAI:
			out, ok = truncateRequestLazy(body, protocol, tc)
			if !ok {
				return body, false
			}
			mergeTruncCtx(info, tc, side)
			return out, true
		default:
			return body, false
		}
	}

	// 响应侧仍走原 map[string]any 路径
	var root any
	if err := sonic.Unmarshal(body, &root); err != nil {
		return body, false
	}

	switch protocol {
	case DebugLogProtocolAnthropic:
		truncateAnthropicResponse(root, tc)
	case DebugLogProtocolOpenAI:
		truncateOpenAIResponse(root, tc)
	default:
		return body, false
	}

	mergeTruncCtx(info, tc, side)

	out, err := sonic.Marshal(root)
	if err != nil {
		return body, false
	}
	return out, true
}

// mergeTruncCtx 把单次 SmartTruncate 收集到的 truncCtx 累加到外部 TruncationInfo。
func mergeTruncCtx(info *TruncationInfo, tc *truncCtx, side debugLogSide) {
	if info == nil {
		return
	}
	if side == debugLogSideRequest {
		info.Request = append(info.Request, tc.cuts...)
	} else {
		info.Response = append(info.Response, tc.cuts...)
	}
	info.ImagesStripped += tc.stripped
	info.ToolResultsCut += tc.toolResults
	info.ToolResultsElided += tc.toolResultsElided
	info.HistoricalMessagesElided += tc.historicalMessagesElided
	info.ToolsSimplified += tc.toolsSimplified
	info.ToolsBytesSaved += tc.toolsBytesSaved
	info.SignaturesStripped += tc.signaturesStripped
	info.CacheControlStripped += tc.cacheControlStripped
}

// truncateRequestLazy 是请求侧的 D2 入口：
//  1. 顶层用 map[string]json.RawMessage 接收，非 messages/tools/system/instructions/input
//     的字段零开销原样保留（[]byte 切片透传，不进 map[string]any 树）。
//  2. messages 数组逐条用 RawMessage 接收，浅层取 role 算出 lastTurnStart。
//  3. 历史消息只浅层解析 role/content 类型做统计；最近一轮才完整 Unmarshal。
//  4. tools 只解析每个元素的 name 字段，schema/description 不进入 map。
func truncateRequestLazy(body []byte, protocol DebugLogProtocol, tc *truncCtx) ([]byte, bool) {
	var top map[string]json.RawMessage
	if err := sonic.Unmarshal(body, &top); err != nil {
		return nil, false
	}

	// system / instructions
	if protocol == DebugLogProtocolAnthropic {
		if raw, ok := top["system"]; ok && len(raw) > 0 {
			var sys any
			if err := sonic.Unmarshal(raw, &sys); err == nil {
				newSys := truncateAnthropicSystem(sys, tc)
				if b, err := sonic.Marshal(newSys); err == nil {
					top["system"] = b
				}
			}
		}
	} else {
		// OpenAI: system / instructions 都是字符串
		if raw, ok := top["system"]; ok && len(raw) > 0 {
			var s string
			if err := sonic.Unmarshal(raw, &s); err == nil {
				ns := truncateText(s, debugLogFieldTextLimit, "system", tc)
				if b, err := sonic.Marshal(ns); err == nil {
					top["system"] = b
				}
			}
		}
		if raw, ok := top["instructions"]; ok && len(raw) > 0 {
			var s string
			if err := sonic.Unmarshal(raw, &s); err == nil {
				ns := truncateText(s, debugLogFieldTextLimit, "instructions", tc)
				if b, err := sonic.Marshal(ns); err == nil {
					top["instructions"] = b
				}
			}
		}
	}

	// tools: 只解析每个元素的 name，schema/description 完全跳过
	if raw, ok := top["tools"]; ok && len(raw) > 0 {
		if simplified, ok := simplifyToolsLazy(raw, protocol, tc); ok {
			top["tools"] = simplified
		}
	}

	// messages（最大头）
	if raw, ok := top["messages"]; ok && len(raw) > 0 {
		if newMsgs, ok := truncateMessagesLazy(raw, protocol, tc); ok {
			top["messages"] = newMsgs
		}
	}

	// OpenAI Responses API: input
	if protocol == DebugLogProtocolOpenAI {
		if raw, ok := top["input"]; ok && len(raw) > 0 {
			if newInput, ok := truncateOpenAIInputLazy(raw, tc); ok {
				top["input"] = newInput
			}
		}
	}

	out, err := sonic.Marshal(top)
	if err != nil {
		return nil, false
	}
	return out, true
}

// simplifyToolsLazy 只解析每个 tool 元素的 name 字段，生成 ["name1", ...]。
// 整个 tool 的 schema/description 不进入 map[string]any，省 90%+ 解析量。
func simplifyToolsLazy(raw json.RawMessage, protocol DebugLogProtocol, tc *truncCtx) (json.RawMessage, bool) {
	var tools []json.RawMessage
	if err := sonic.Unmarshal(raw, &tools); err != nil || len(tools) == 0 {
		return nil, false
	}
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = extractToolNameLazy(t, protocol)
		if names[i] == "" {
			names[i] = "<unnamed>"
		}
	}
	simplified, err := sonic.Marshal(names)
	if err != nil {
		return nil, false
	}
	tc.toolsSimplified += len(tools)
	if saved := len(raw) - len(simplified); saved > 0 {
		tc.toolsBytesSaved += saved
	}
	return simplified, true
}

// extractToolNameLazy 浅层提取 tool 的 name，不解析 schema/description。
func extractToolNameLazy(raw json.RawMessage, protocol DebugLogProtocol) string {
	if protocol == DebugLogProtocolAnthropic {
		var t struct {
			Name string `json:"name"`
		}
		_ = sonic.Unmarshal(raw, &t)
		return t.Name
	}
	// OpenAI: chat.completions {type:"function", function:{name}} 或
	// Responses API {type, name}
	var t struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	_ = sonic.Unmarshal(raw, &t)
	if t.Function.Name != "" {
		return t.Function.Name
	}
	if t.Name != "" {
		return t.Name
	}
	return t.Type
}

// truncateMessagesLazy 是 D2 的核心收益点：
// 历史消息只浅层提取 role/content 做统计后整段丢弃（聚合成一条占位）；
// 最近一轮才完整 Unmarshal 到 map[string]any 走原 truncateXxxMessageContent。
func truncateMessagesLazy(raw json.RawMessage, protocol DebugLogProtocol, tc *truncCtx) (json.RawMessage, bool) {
	var msgs []json.RawMessage
	if err := sonic.Unmarshal(raw, &msgs); err != nil {
		return nil, false
	}
	if len(msgs) == 0 {
		return raw, true
	}

	// 浅层取每条 role 找 lastTurnStart
	roles := make([]string, len(msgs))
	for i, m := range msgs {
		var named struct {
			Role string `json:"role"`
		}
		_ = sonic.Unmarshal(m, &named)
		roles[i] = named.Role
	}
	lastTurnStart := findLastTurnStartFromRoles(roles)

	// 输出 = 历史聚合占位 + 最近一轮逐条改写
	newMsgs := make([]json.RawMessage, 0, len(msgs)-lastTurnStart+1)

	if lastTurnStart > 0 {
		summary := aggregateHistoricalMessagesLazy(msgs[:lastTurnStart], tc)
		if summary != nil {
			if b, err := sonic.Marshal(summary); err == nil {
				newMsgs = append(newMsgs, b)
			}
		}
	}

	for i := lastTurnStart; i < len(msgs); i++ {
		var msgMap map[string]any
		if err := sonic.Unmarshal(msgs[i], &msgMap); err != nil {
			newMsgs = append(newMsgs, msgs[i])
			continue
		}
		path := fmt.Sprintf("messages[%d]", i)
		switch protocol {
		case DebugLogProtocolAnthropic:
			truncateAnthropicMessageContent(msgMap, path, tc, false)
		case DebugLogProtocolOpenAI:
			truncateOpenAIMessage(msgMap, path, tc, false)
		}
		if b, err := sonic.Marshal(msgMap); err == nil {
			newMsgs = append(newMsgs, b)
		} else {
			newMsgs = append(newMsgs, msgs[i])
		}
	}

	out, err := sonic.Marshal(newMsgs)
	if err != nil {
		return nil, false
	}
	return out, true
}

// findLastTurnStartFromRoles 用浅层 role 数组定位最近一轮起点。
// 语义与 findLastTurnStart 一致：倒数第二个 user 之后。
func findLastTurnStartFromRoles(roles []string) int {
	userIdxs := make([]int, 0, 2)
	for i, r := range roles {
		if r == "user" {
			userIdxs = append(userIdxs, i)
		}
	}
	if len(userIdxs) < 2 {
		return 0
	}
	return userIdxs[len(userIdxs)-2] + 1
}

// aggregateHistoricalMessagesLazy 对每条历史消息只做浅层解析：role + content
// 的类型分布（string 或 block array 的 type 计数），不深入 content 内部文本/
// 图片字节，输出与 aggregateHistoricalMessages 格式一致的占位消息。
func aggregateHistoricalMessagesLazy(msgs []json.RawMessage, tc *truncCtx) map[string]any {
	if len(msgs) == 0 {
		return nil
	}
	userCount, assistantCount, otherCount := 0, 0, 0
	blockTypes := map[string]int{}
	textMessages := 0
	for _, raw := range msgs {
		var shallow struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := sonic.Unmarshal(raw, &shallow); err != nil {
			otherCount++
			tc.historicalMessagesElided++
			continue
		}
		switch shallow.Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		default:
			otherCount++
		}
		if len(shallow.Content) > 0 {
			var s string
			if err := sonic.Unmarshal(shallow.Content, &s); err == nil {
				if s != "" {
					textMessages++
				}
			} else {
				var blocks []json.RawMessage
				if err := sonic.Unmarshal(shallow.Content, &blocks); err == nil {
					for _, blk := range blocks {
						var typed struct {
							Type string `json:"type"`
						}
						_ = sonic.Unmarshal(blk, &typed)
						t := typed.Type
						if t == "" {
							t = "unknown"
						}
						blockTypes[t]++
					}
				}
			}
		}
		tc.historicalMessagesElided++
	}
	return renderHistoricalSummary(len(msgs), userCount, assistantCount, otherCount, textMessages, blockTypes)
}

// renderHistoricalSummary 渲染聚合占位消息，与 aggregateHistoricalMessages
// 输出格式保持一致以便前端兼容。
func renderHistoricalSummary(total, userCount, assistantCount, otherCount, textMessages int, blockTypes map[string]int) map[string]any {
	var b strings.Builder
	fmt.Fprintf(&b, "<historical %d message(s) elided: ", total)
	parts := make([]string, 0, 4)
	if userCount > 0 {
		parts = append(parts, fmt.Sprintf("%d user", userCount))
	}
	if assistantCount > 0 {
		parts = append(parts, fmt.Sprintf("%d assistant", assistantCount))
	}
	if otherCount > 0 {
		parts = append(parts, fmt.Sprintf("%d other", otherCount))
	}
	b.WriteString(strings.Join(parts, ", "))
	if textMessages > 0 || len(blockTypes) > 0 {
		b.WriteString("; blocks: ")
		blockParts := make([]string, 0, len(blockTypes)+1)
		if textMessages > 0 {
			blockParts = append(blockParts, fmt.Sprintf("%d text-message", textMessages))
		}
		keys := make([]string, 0, len(blockTypes))
		for k := range blockTypes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			blockParts = append(blockParts, fmt.Sprintf("%d %s", blockTypes[k], k))
		}
		b.WriteString(strings.Join(blockParts, ", "))
	}
	b.WriteString(">")
	return map[string]any{
		"role":    "system",
		"content": b.String(),
	}
}

// truncateOpenAIInputLazy 处理 Responses API 的 input：string 或数组。
// 数组元素结构同 messages，复用 truncateOpenAIMessage。
func truncateOpenAIInputLazy(raw json.RawMessage, tc *truncCtx) (json.RawMessage, bool) {
	var s string
	if err := sonic.Unmarshal(raw, &s); err == nil {
		ns := truncateText(s, debugLogFieldTextLimit, "input", tc)
		if b, err := sonic.Marshal(ns); err == nil {
			return b, true
		}
		return nil, false
	}
	var arr []json.RawMessage
	if err := sonic.Unmarshal(raw, &arr); err != nil {
		return nil, false
	}
	out := make([]json.RawMessage, len(arr))
	for i, item := range arr {
		var m map[string]any
		if err := sonic.Unmarshal(item, &m); err != nil {
			out[i] = item
			continue
		}
		truncateOpenAIMessage(m, fmt.Sprintf("input[%d]", i), tc, false)
		if b, err := sonic.Marshal(m); err == nil {
			out[i] = b
		} else {
			out[i] = item
		}
	}
	b, err := sonic.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
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

// stripCacheControl 删除 message / content block / system 块上的 cache_control 标记。
// 调试日志无需关心客户端的缓存策略。
func stripCacheControl(m map[string]any, tc *truncCtx) {
	if _, ok := m["cache_control"]; ok {
		delete(m, "cache_control")
		tc.cacheControlStripped++
	}
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
		if err := sonic.Unmarshal([]byte(pj), &parsed); err == nil {
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

