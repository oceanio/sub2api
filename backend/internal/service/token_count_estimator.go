package service

import (
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

var (
	tiktokenOnce sync.Once
	tiktokenEnc  *tiktoken.Tiktoken
	tiktokenErr  error
)

func getSharedTiktokenEncoding() (*tiktoken.Tiktoken, error) {
	tiktokenOnce.Do(func() {
		tiktokenEnc, tiktokenErr = tiktoken.GetEncoding("cl100k_base")
	})
	return tiktokenEnc, tiktokenErr
}

// estimateInputTokens estimates input token count from a parsed request using
// tiktoken cl100k_base encoding (close to Claude's tokenizer). Falls back to
// rune-based estimation if tiktoken is unavailable.
func estimateInputTokens(parsed *ParsedRequest) int {
	var buf strings.Builder
	extractTokenEstimatorText(parsed.System, &buf)
	extractTokenEstimatorText(parsed.Messages, &buf)
	text := buf.String()

	enc, err := getSharedTiktokenEncoding()
	if err != nil {
		// ~3 runes per token on average for mixed Chinese/English
		n := len([]rune(text)) / 3
		if n < 1 {
			return 1
		}
		return n
	}
	tokens := enc.Encode(text, nil, nil)
	if len(tokens) < 1 {
		return 1
	}
	return len(tokens)
}

// extractTokenEstimatorText recursively extracts meaningful text content
// from Anthropic message/system structures into buf.
func extractTokenEstimatorText(v any, buf *strings.Builder) {
	if v == nil {
		return
	}
	switch val := v.(type) {
	case string:
		buf.WriteString(val)
		buf.WriteByte('\n')
	case map[string]any:
		// text block: {"type": "text", "text": "..."}
		if text, ok := val["text"].(string); ok {
			buf.WriteString(text)
			buf.WriteByte('\n')
		}
		// message content or tool_result content
		if content, ok := val["content"]; ok {
			extractTokenEstimatorText(content, buf)
		}
		// tool_use input arguments
		if input, ok := val["input"]; ok {
			extractTokenEstimatorText(input, buf)
		}
	case []any:
		for _, item := range val {
			extractTokenEstimatorText(item, buf)
		}
	}
}
