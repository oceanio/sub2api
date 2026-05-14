package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// responseCapture wraps gin.ResponseWriter to tee response bytes into a
// bounded buffer for debug logging. Writes always pass through to the
// underlying writer so the client is never affected.
type responseCapture struct {
	gin.ResponseWriter
	buf   []byte
	limit int
}

func newResponseCapture(w gin.ResponseWriter, limit int) *responseCapture {
	return &responseCapture{ResponseWriter: w, limit: limit}
}

func (r *responseCapture) Write(b []byte) (int, error) {
	if len(r.buf) < r.limit {
		remaining := r.limit - len(r.buf)
		if len(b) <= remaining {
			r.buf = append(r.buf, b...)
		} else {
			r.buf = append(r.buf, b[:remaining]...)
		}
	}
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteString(s string) (int, error) {
	return r.Write([]byte(s))
}

func (r *responseCapture) captured() []byte {
	return r.buf
}

// enqueueDebugLog builds a debug log entry and pushes it to the service queue.
// Shared by Anthropic gateway handler and OpenAI gateway handler.
//
// svc 为 nil 时无操作；调用方应在 Forward 成功后调用，并通过 protocol 告知
// 上游协议以便流式响应能被正确聚合。
func enqueueDebugLog(
	svc *service.RequestDebugLogService,
	ctx context.Context,
	requestID string,
	apiKey *service.APIKey,
	model string,
	stream bool,
	protocol service.DebugLogProtocol,
	headers http.Header,
	reqBody []byte,
	respBody []byte,
) {
	if svc == nil {
		return
	}
	if requestID == "" {
		requestID, _ = ctx.Value(ctxkey.RequestID).(string)
	}
	if requestID == "" {
		// 上游与中间件都未提供 request_id 时本地生成，dbg- 前缀便于识别降级路径。
		requestID = "dbg-" + uuid.NewString()
	}
	var userID, apiKeyID, groupID *int64
	if apiKey != nil {
		id := apiKey.ID
		apiKeyID = &id
		groupID = apiKey.GroupID
		if apiKey.User != nil {
			uid := apiKey.User.ID
			userID = &uid
		}
	}
	entry := svc.BuildEntry(ctx, requestID, userID, apiKeyID, groupID, model, stream, protocol, headers, reqBody, respBody, "")
	svc.Enqueue(entry)
}
