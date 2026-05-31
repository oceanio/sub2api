// Fork: 透传路径补齐 Forward 主路径已有的 invalid_encrypted_content 同账号清洗重试逻辑。
// 镜像 openai_gateway_service.go 中 WS 路径 recoverInvalidEncryptedContent 的 trim + previous_response_id drop 行为。
package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// tryOpenAIPassthroughInvalidEncryptedRecovery 在透传路径上对 400 invalid_encrypted_content
// 做一次清洗+重试准备：剥掉 input 里的 reasoning.encrypted_content，并在不含
// function_call_output 时一并 drop previous_response_id；返回清洗后的 body 和是否应重试。
// 与 Forward 主路径（line ~3053）+ WS 路径（line ~2844）保持等价行为。
func tryOpenAIPassthroughInvalidEncryptedRecovery(body []byte, respBody []byte, accountName string) ([]byte, bool) {
	if extractUpstreamErrorCode(respBody) != "invalid_encrypted_content" {
		return nil, false
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return nil, false
	}
	if !trimOpenAIEncryptedReasoningItems(reqBody) {
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI 自动透传] Skip invalid_encrypted_content retry because encrypted reasoning items are missing (account: %s)",
			accountName)
		return nil, false
	}
	if prevID := strings.TrimSpace(openAIWSPayloadString(reqBody, "previous_response_id")); prevID != "" && !HasFunctionCallOutput(reqBody) {
		delete(reqBody, "previous_response_id")
	}
	nextBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, false
	}
	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI 自动透传] Retrying once after invalid_encrypted_content (account: %s)",
		accountName)
	return nextBody, true
}
