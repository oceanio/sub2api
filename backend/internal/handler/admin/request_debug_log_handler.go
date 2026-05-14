package admin

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// RequestDebugLogHandler 请求调试日志管理接口
type RequestDebugLogHandler struct {
	debugLogRepo service.RequestDebugLogRepository
}

func NewRequestDebugLogHandler(debugLogRepo service.RequestDebugLogRepository) *RequestDebugLogHandler {
	return &RequestDebugLogHandler{debugLogRepo: debugLogRepo}
}

// DebugLogResponse GET /api/v1/admin/usage/:request_id/debug-log 的响应结构
type DebugLogResponse struct {
	Found          bool               `json:"found"`
	RequestID      string             `json:"request_id,omitempty"`
	Model          string             `json:"model,omitempty"`
	Stream         bool               `json:"stream,omitempty"`
	RequestHeaders map[string]any     `json:"request_headers,omitempty"`
	RequestBody    any                `json:"request_body,omitempty"`
	ResponseBody   any                `json:"response_body,omitempty"`
	ResponseText   string             `json:"response_text,omitempty"`
	Truncated      bool               `json:"truncated,omitempty"`
	BodyBytes      int                `json:"body_bytes,omitempty"`
	CreatedAt      *time.Time         `json:"created_at,omitempty"`
	ExpiresAt      *time.Time         `json:"expires_at,omitempty"`
}

// GetByRequestID GET /api/v1/admin/usage/:request_id/debug-log
func (h *RequestDebugLogHandler) GetByRequestID(c *gin.Context) {
	requestID := c.Param("request_id")
	if requestID == "" {
		response.BadRequest(c, "request_id is required")
		return
	}

	log, err := h.debugLogRepo.GetByRequestID(c.Request.Context(), requestID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if log == nil {
		response.Success(c, DebugLogResponse{Found: false})
		return
	}

	resp := DebugLogResponse{
		Found:      true,
		RequestID:  log.RequestID,
		Model:      log.Model,
		Stream:     log.Stream,
		Truncated:  log.Truncated,
		BodyBytes:  log.BodyBytes,
		CreatedAt:  &log.CreatedAt,
		ExpiresAt:  &log.ExpiresAt,
		ResponseText: log.ResponseText,
	}

	if log.RequestHeaders != nil {
		var headers map[string]any
		if err := json.Unmarshal(log.RequestHeaders, &headers); err == nil {
			resp.RequestHeaders = headers
		}
	}
	if log.RequestBody != nil {
		var body any
		if err := json.Unmarshal(log.RequestBody, &body); err == nil {
			resp.RequestBody = body
		}
	}
	if log.ResponseBody != nil {
		var body any
		if err := json.Unmarshal(log.ResponseBody, &body); err == nil {
			resp.ResponseBody = body
		}
	}

	response.Success(c, resp)
}
