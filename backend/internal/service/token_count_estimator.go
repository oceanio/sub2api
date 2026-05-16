package service

import (
	"encoding/json"
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

var (
	tiktokenOnce sync.Once
	tiktokenEnc  *tiktoken.Tiktoken
	tiktokenErr  error
)

// claudeImageTokenEstimate 是 Claude 协议图片块的保守 token 估算值。
// 官方公式 tokens = (width * height) / 750，单张上限 ~1600。
// fallback 路径无法获取图片实际尺寸（base64 解码代价过高），
// 用 1500 作为接近上限的保守值，避免严重低估多模态请求成本。
const claudeImageTokenEstimate = 1500

func getSharedTiktokenEncoding() (*tiktoken.Tiktoken, error) {
	tiktokenOnce.Do(func() {
		tiktokenEnc, tiktokenErr = tiktoken.GetEncoding("cl100k_base")
	})
	return tiktokenEnc, tiktokenErr
}

// estimateInputTokens estimates input token count from a parsed request using
// tiktoken cl100k_base encoding (close to Claude's tokenizer). Falls back to
// rune-based estimation if tiktoken is unavailable. Image blocks are counted
// with a conservative per-image estimate since fallback path cannot decode
// image dimensions cheaply.
func estimateInputTokens(parsed *ParsedRequest) int {
	var buf strings.Builder
	imageCount := 0
	extractTokenEstimatorText(parsed.System, &buf, &imageCount)

	// Anthropic 行为:历史 assistant 轮次的 thinking 块不进上下文/计费,
	// 仅"当前轮次"(即最后一条 role=assistant 的消息)的 thinking 计入。
	// 这里据此区分:
	//   - 最后一条 assistant 消息:thinking 文本计入 buf
	//   - 其他所有消息:thinking 块整个忽略
	//   - redacted_thinking:加密内容非自然语言,任何位置都不计入
	lastAssistantIdx := -1
	for i, msg := range parsed.Messages {
		if m, ok := msg.(map[string]any); ok {
			if r, _ := m["role"].(string); r == "assistant" {
				lastAssistantIdx = i
			}
		}
	}
	for i, msg := range parsed.Messages {
		extractMessageTokenText(msg, i == lastAssistantIdx, &buf, &imageCount)
	}

	appendToolDefinitionsText(parsed.Body, &buf)
	text := buf.String()

	textTokens := 0
	enc, err := getSharedTiktokenEncoding()
	if err != nil {
		// ~3 runes per token on average for mixed Chinese/English
		textTokens = len([]rune(text)) / 3
	} else {
		textTokens = len(enc.Encode(text, nil, nil))
	}

	total := textTokens + imageCount*claudeImageTokenEstimate
	if total < 1 {
		return 1
	}
	return total
}

// appendToolDefinitionsText 解析请求体顶层 tools 数组，把每个工具定义
// （name + description + input_schema）并入 buf 参与 token 估算。
// 本地估算路径（上游 5xx / Bedrock / Antigravity 回退）原本只算
// system+messages，而 Claude Code 一次会带几十个工具、schema 动辄上千
// token，不计入会严重低估输入。input_schema 用紧凑 JSON 序列化作为
// 文本代理（含结构键），与上游官方计费口径更接近。
func appendToolDefinitionsText(body []byte, buf *strings.Builder) {
	if len(body) == 0 {
		return
	}
	var root struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(body, &root); err != nil || len(root.Tools) == 0 {
		return
	}
	for _, tool := range root.Tools {
		if name, ok := tool["name"].(string); ok {
			buf.WriteString(name)
			buf.WriteByte('\n')
		}
		if desc, ok := tool["description"].(string); ok {
			buf.WriteString(desc)
			buf.WriteByte('\n')
		}
		if schema, ok := tool["input_schema"]; ok {
			if b, err := json.Marshal(schema); err == nil {
				buf.Write(b)
				buf.WriteByte('\n')
			}
		}
	}
}

// extractMessageTokenText 处理单条 message,对 thinking 块做轮次相关的取舍:
// 只在 isCurrentAssistant=true 时把 thinking 文本计入估算;其他位置的
// thinking / 任意位置的 redacted_thinking 全部跳过。其它内容(text / image /
// tool_use input / tool_result content)走通用递归。
func extractMessageTokenText(msg any, isCurrentAssistant bool, buf *strings.Builder, images *int) {
	m, ok := msg.(map[string]any)
	if !ok {
		extractTokenEstimatorText(msg, buf, images)
		return
	}
	content, hasContent := m["content"]
	if !hasContent {
		return
	}
	// content 可能是字符串(简单消息)或 block 数组
	blocks, ok := content.([]any)
	if !ok {
		extractTokenEstimatorText(content, buf, images)
		return
	}
	for _, item := range blocks {
		blk, ok := item.(map[string]any)
		if !ok {
			extractTokenEstimatorText(item, buf, images)
			continue
		}
		switch blk["type"] {
		case "thinking":
			if !isCurrentAssistant {
				continue
			}
			if txt, ok := blk["thinking"].(string); ok {
				buf.WriteString(txt)
				buf.WriteByte('\n')
			}
		case "redacted_thinking":
			// 加密内容,任何轮次都不计入
			continue
		default:
			extractTokenEstimatorText(blk, buf, images)
		}
	}
}

// extractTokenEstimatorText recursively extracts meaningful text content
// from Anthropic message/system structures into buf, and counts image blocks.
func extractTokenEstimatorText(v any, buf *strings.Builder, images *int) {
	if v == nil {
		return
	}
	switch val := v.(type) {
	case string:
		buf.WriteString(val)
		buf.WriteByte('\n')
	case map[string]any:
		// image block: {"type": "image", "source": {...}}
		if t, ok := val["type"].(string); ok && t == "image" {
			if images != nil {
				*images++
			}
			return
		}
		// text block: {"type": "text", "text": "..."}
		if text, ok := val["text"].(string); ok {
			buf.WriteString(text)
			buf.WriteByte('\n')
		}
		// message content or tool_result content
		if content, ok := val["content"]; ok {
			extractTokenEstimatorText(content, buf, images)
		}
		// tool_use input arguments
		if input, ok := val["input"]; ok {
			extractTokenEstimatorText(input, buf, images)
		}
	case []any:
		for _, item := range val {
			extractTokenEstimatorText(item, buf, images)
		}
	}
}
