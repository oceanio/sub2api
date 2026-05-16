package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// RequestDebugLog 单条调试日志记录
type RequestDebugLog struct {
	ID              int64
	RequestID       string
	UserID          *int64
	APIKeyID        *int64
	GroupID         *int64
	Model           string
	Stream          bool
	RequestHeaders  json.RawMessage
	RequestBody     json.RawMessage
	RequestText     string          // 字段级截断不可行时的请求兜底文本
	ResponseHeaders json.RawMessage
	ResponseBody    json.RawMessage
	ResponseText    string
	Truncated       bool
	TruncationInfo  json.RawMessage // 结构化截断元信息(被截字段、字节数、剥离的图片数等)
	BodyBytes       int
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// RequestDebugLogRepository 持久化接口
type RequestDebugLogRepository interface {
	BatchInsert(ctx context.Context, logs []*RequestDebugLog) error
	GetByRequestID(ctx context.Context, requestID string) (*RequestDebugLog, error)
	DeleteExpired(ctx context.Context, now time.Time, batchSize int) (int64, error)
}

const (
	debugLogWorkerCount   = 2
	debugLogQueueSize     = 1000
	debugLogBatchSize     = 50
	debugLogFlushInterval = 2 * time.Second
	debugLogMaxBodyBytes  = 1024 * 1024 // 1 MB 硬上限(Anthropic 已支持 1M context)
	debugLogCleanupEvery  = 10 * time.Minute
	debugLogCleanupBatch  = 5000
)

// RequestDebugLogService 异步写入调试日志
type RequestDebugLogService struct {
	repo           RequestDebugLogRepository
	settingService *SettingService
	queue          chan *RequestDebugLog
	wg             sync.WaitGroup
	stopCh         chan struct{}
}

func NewRequestDebugLogService(
	repo RequestDebugLogRepository,
	settingService *SettingService,
) *RequestDebugLogService {
	return &RequestDebugLogService{
		repo:           repo,
		settingService: settingService,
		queue:          make(chan *RequestDebugLog, debugLogQueueSize),
		stopCh:         make(chan struct{}),
	}
}

// ShouldRecord 检查全局开关 + 采样率，主链路调用（轻量）
func (s *RequestDebugLogService) ShouldRecord(ctx context.Context) bool {
	if !s.settingService.IsDebugRequestLogEnabled(ctx) {
		return false
	}
	rate := s.settingService.GetDebugRequestLogSampleRate(ctx)
	if rate >= 100 {
		return true
	}
	return rand.IntN(100) < rate
}

// Enqueue 投入队列；队列满则丢弃，不阻塞主流程
func (s *RequestDebugLogService) Enqueue(log *RequestDebugLog) {
	if log == nil {
		return
	}
	select {
	case s.queue <- log:
	default:
		slog.Warn("debug_log_queue_full_dropped", "request_id", log.RequestID)
	}
}

// BuildEntry 根据当前 settings 构建 RequestDebugLog 条目
// 由 gateway handler 在成功路径上调用
//
// stream=true 时根据 protocol 把捕获到的 SSE 字节聚合为最终 JSON 存入 response_body；
// 聚合失败/不支持的协议会回退到 response_text 存储原始 SSE 文本。
func (s *RequestDebugLogService) BuildEntry(
	ctx context.Context,
	requestID string,
	userID *int64,
	apiKeyID *int64,
	groupID *int64,
	model string,
	stream bool,
	protocol DebugLogProtocol,
	headers http.Header,
	requestBody []byte,
	responseHeaders http.Header,
	responseBody []byte,
	responseText string,
) *RequestDebugLog {
	redact := s.settingService.IsDebugRequestLogRedactHeaders(ctx)
	ttl := s.settingService.GetDebugRequestLogTTL(ctx)
	now := time.Now()

	entry := &RequestDebugLog{
		RequestID: requestID,
		UserID:    userID,
		APIKeyID:  apiKeyID,
		GroupID:   groupID,
		Model:     model,
		Stream:    stream,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	// 请求头
	if headers != nil {
		sanitized := SanitizeHeaders(headers, redact)
		if b, err := json.Marshal(sanitized); err == nil {
			entry.RequestHeaders = b
		}
	}

	// 响应头
	if responseHeaders != nil {
		sanitized := SanitizeHeaders(responseHeaders, redact)
		if b, err := json.Marshal(sanitized); err == nil {
			entry.ResponseHeaders = b
		}
	}

	originalLen := len(requestBody) + len(responseBody)
	entry.BodyBytes = originalLen

	captureTrunc := len(responseBody) >= debugLogMaxBodyBytes
	info := &TruncationInfo{}

	// 请求体:走字段级智能截断;失败兜底到 request_text
	if len(requestBody) > 0 {
		applyBodyToEntry(requestBody, protocol, debugLogSideRequest, info, entry, false)
	}

	// 流式响应:先尝试 SSE 聚合;失败标记 AggregationFailed,后续走原始字节
	streamAggregated := false
	if stream && len(responseBody) > 0 {
		if aggregated, err := AggregateStream(protocol, responseBody); err == nil && len(aggregated) > 0 {
			responseBody = aggregated
			streamAggregated = true
		} else {
			info.AggregationFailed = true
		}
	}

	// 响应体:字段级智能截断;聚合失败/非 JSON 兜底到 response_text
	switch {
	case len(responseBody) > 0:
		// 聚合失败(原始 SSE)走字节兜底,不尝试 SmartTruncate
		forceText := stream && !streamAggregated
		applyBodyToEntry(responseBody, protocol, debugLogSideResponse, info, entry, forceText)
	case responseText != "":
		t, _ := TruncateBody([]byte(responseText), 0, debugLogMaxBodyBytes)
		entry.ResponseText = string(t)
	}

	entry.Truncated = captureTrunc || info.hasAny()
	if info.hasAny() {
		if b, err := json.Marshal(info); err == nil {
			entry.TruncationInfo = b
		}
	}

	return entry
}

// applyBodyToEntry 对一段 body(请求或响应)做"字段级智能截断 + 字节硬上限兜底",
// 把结果写到 entry 的对应字段。
// forceText=true 表示已知不是 JSON(如流式聚合失败),直接走字节硬截 + Text 列。
func applyBodyToEntry(body []byte, protocol DebugLogProtocol, side debugLogSide, info *TruncationInfo, entry *RequestDebugLog, forceText bool) {
	assign := func(jsonBody json.RawMessage, text string) {
		if side == debugLogSideRequest {
			entry.RequestBody = jsonBody
			if text != "" {
				entry.RequestText = text
			}
		} else {
			entry.ResponseBody = jsonBody
			if text != "" {
				entry.ResponseText = text
			}
		}
	}

	if forceText {
		t, _ := TruncateBody(body, 0, debugLogMaxBodyBytes)
		assign(nil, string(t))
		return
	}

	smart, ok := SmartTruncate(body, protocol, side, info)
	if !ok {
		info.SmartFailed = true
		t, _ := TruncateBody(body, 0, debugLogMaxBodyBytes)
		assign(nil, string(t))
		return
	}

	// 字段级截断后若仍超硬上限(极少数:几十张图、几百轮历史),再做字节硬截。
	// 此时 JSON 结构很可能被破坏 → 退化到 Text 列。
	if len(smart) > debugLogMaxBodyBytes {
		info.OverallCutBytes = len(smart) - debugLogMaxBodyBytes
		t, _ := TruncateBody(smart, 0, debugLogMaxBodyBytes)
		if json.Valid(t) {
			assign(json.RawMessage(t), "")
		} else {
			assign(nil, string(t))
		}
		return
	}

	assign(json.RawMessage(smart), "")
}

// MaxBodyBytes returns the hard upper limit for response capture.
func (s *RequestDebugLogService) MaxBodyBytes() int {
	return debugLogMaxBodyBytes
}

// Start 启动后台 worker
func (s *RequestDebugLogService) Start() {
	for i := 0; i < debugLogWorkerCount; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	s.wg.Add(1)
	go s.cleanup()
}

// Stop 优雅停止，等待 worker 消费完队列
func (s *RequestDebugLogService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *RequestDebugLogService) worker() {
	defer s.wg.Done()
	batch := make([]*RequestDebugLog, 0, debugLogBatchSize)
	ticker := time.NewTicker(debugLogFlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.repo.BatchInsert(flushCtx, batch); err != nil {
			slog.Error("debug_log_batch_insert_failed", "count", len(batch), "error", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case log, ok := <-s.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, log)
			if len(batch) >= debugLogBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.stopCh:
			// 消费剩余
		drain:
			for {
				select {
				case log := <-s.queue:
					batch = append(batch, log)
				default:
					break drain
				}
			}
			flush()
			return
		}
	}
}

func (s *RequestDebugLogService) cleanup() {
	defer s.wg.Done()
	ticker := time.NewTicker(debugLogCleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			deleted, err := s.repo.DeleteExpired(cleanCtx, time.Now(), debugLogCleanupBatch)
			cancel()
			if err != nil {
				slog.Error("debug_log_cleanup_failed", "error", err)
			} else if deleted > 0 {
				slog.Info("debug_log_cleanup_done", "deleted", deleted)
			}
		case <-s.stopCh:
			return
		}
	}
}

// SanitizeHeaders 按 redact 决定是否脱敏敏感 header
func SanitizeHeaders(h http.Header, redact bool) map[string]string {
	sensitiveHeaders := map[string]bool{
		"authorization":      true,
		"x-api-key":          true,
		"api-key":            true,
		"anthropic-api-key":  true,
		"openai-api-key":     true,
		"x-goog-api-key":     true,
		"cookie":             true,
		"set-cookie":         true,
	}

	result := make(map[string]string, len(h))
	for k, vals := range h {
		v := strings.Join(vals, ", ")
		if redact && sensitiveHeaders[strings.ToLower(k)] {
			if len(v) > 4 {
				v = v[:4] + "***"
			} else {
				v = "***"
			}
		}
		result[k] = v
	}
	return result
}

// TruncateBody 对 body 按字节截断（UTF-8 安全）
// softLimit=0 表示不做软截断；hardLimit 始终生效
// 返回 (截断后内容, 是否发生截断)
func TruncateBody(body []byte, softLimit, hardLimit int) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	limit := hardLimit
	if softLimit > 0 && softLimit < hardLimit {
		limit = softLimit
	}
	if limit <= 0 || len(body) <= limit {
		return body, false
	}
	// UTF-8 安全截断：不在多字节字符中间切割
	cut := limit
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut], true
}

