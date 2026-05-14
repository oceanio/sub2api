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
	extractTokenEstimatorText(parsed.Messages, &buf, &imageCount)
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
