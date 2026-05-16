package handler

import (
	"context"
	"log/slog"
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

// shouldDebugLog checks whether debug logging should be activated for this
// request. Any panic inside ShouldRecord is caught and treated as "no" so the
// debug-log feature can never affect the main request path.
func shouldDebugLog(svc *service.RequestDebugLogService, ctx context.Context) bool {
	if svc == nil {
		return false
	}
	ok := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("debug_log_should_record_panic", "recover", r)
			}
		}()
		ok = svc.ShouldRecord(ctx)
	}()
	return ok
}

// enqueueDebugLog builds a debug log entry and pushes it to the service queue.
// Shared by Anthropic gateway handler and OpenAI gateway handler.
//
// All work (BuildEntry + Enqueue) runs in a detached goroutine with panic
// recovery so that any failure in the debug-log path never affects the
// already-committed LLM response. context.Background() is used because the
// request context may already be cancelled by the time the goroutine runs.
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
	respHeaders http.Header,
	respBody []byte,
) {
	if svc == nil {
		return
	}

	if requestID == "" {
		requestID, _ = ctx.Value(ctxkey.RequestID).(string)
	}
	if requestID == "" {
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

	// 在主线程克隆 header，避免 gin 复用 Context / net/http 复用 conn 时
	// 在 goroutine 内访问到被重置的 header map。
	var hdrClone, respHdrClone http.Header
	if headers != nil {
		hdrClone = headers.Clone()
	}
	if respHeaders != nil {
		respHdrClone = respHeaders.Clone()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("debug_log_enqueue_panic", "request_id", requestID, "recover", r)
			}
		}()
		entry := svc.BuildEntry(context.Background(), requestID, userID, apiKeyID, groupID, model, stream, protocol, hdrClone, reqBody, respHdrClone, respBody, "")
		svc.Enqueue(entry)
	}()
}
