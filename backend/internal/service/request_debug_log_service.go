package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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
	debugLogBatchBytes    = 8 * 1024 * 1024 // 8 MB 单 batch 字节上限，避免 1MB×50=50MB 巨型事务
	debugLogFlushInterval = 2 * time.Second
	debugLogMaxBodyBytes  = 1024 * 1024 // 1 MB 硬上限(Anthropic 已支持 1M context)
	debugLogCleanupEvery  = 10 * time.Minute
	debugLogCleanupBatch  = 5000

	// 反压相关
	// debugLogBuildConcurrency 限制并发执行 BuildEntry 的 goroutine 数。
	// BuildEntry 内会跑 JSON Unmarshal + 树遍历 + Marshal，单条最大 1MB
	// JSON 在 map[string]any 下膨胀 3-5x；32 是经验值，约对应满载 32 核。
	debugLogBuildConcurrency = 32
	// debugLogQueueHighWatermark 队列高水位（占容量比例的分子/分母）。
	// 超过水位时 ShouldRecord 直接拒绝新采样，避免主路径继续投入工作量。
	debugLogQueueHighWatermarkNum = 8
	debugLogQueueHighWatermarkDen = 10
	// debugLogQueueMaxBytes 队列累计字节硬上限：1000 槽 × 2MB 最坏 = 2GB，
	// 实测平均值小很多但缺少兜底。256MB 封死最坏情况，超过即丢弃 + 告警。
	debugLogQueueMaxBytes = 256 * 1024 * 1024
)

// estimateSize 估算单条 RequestDebugLog 序列化后的字节数，用于 batch 字节配额。
// 取主要 body 字段长度即可，header / 元信息相对很小可忽略。
func (l *RequestDebugLog) estimateSize() int {
	if l == nil {
		return 0
	}
	return len(l.RequestBody) + len(l.ResponseBody) +
		len(l.RequestText) + len(l.ResponseText) +
		len(l.RequestHeaders) + len(l.ResponseHeaders) +
		len(l.TruncationInfo)
}

// RequestDebugLogService 异步写入调试日志
type RequestDebugLogService struct {
	repo           RequestDebugLogRepository
	settingService *SettingService
	queue          chan *RequestDebugLog
	// buildSem 限制并发跑 BuildEntry 的 goroutine 数，避免突发流量打爆 CPU/GC
	buildSem chan struct{}
	// queueBytes 队列中所有待写入条目累计字节数（粗略估算）。
	// Enqueue 时 Add；worker 取出时 Sub。配合 debugLogQueueMaxBytes 封顶。
	queueBytes atomic.Int64
	wg         sync.WaitGroup
	stopCh     chan struct{}
}

func NewRequestDebugLogService(
	repo RequestDebugLogRepository,
	settingService *SettingService,
) *RequestDebugLogService {
	return &RequestDebugLogService{
		repo:           repo,
		settingService: settingService,
		queue:          make(chan *RequestDebugLog, debugLogQueueSize),
		buildSem:       make(chan struct{}, debugLogBuildConcurrency),
		stopCh:         make(chan struct{}),
	}
}

// ShouldRecord 检查全局开关 + 采样率，主链路调用（轻量）。
// 高水位时直接拒绝采样，避免主路径继续投入 responseCapture / BuildEntry 等工作。
func (s *RequestDebugLogService) ShouldRecord(ctx context.Context) bool {
	if !s.settingService.IsDebugRequestLogEnabled(ctx) {
		return false
	}
	// F1 反压：queue 水位 >80% 时直接拒绝。len(chan) 是 O(1) 原子读，热路径可承受。
	if len(s.queue)*debugLogQueueHighWatermarkDen >= debugLogQueueSize*debugLogQueueHighWatermarkNum {
		return false
	}
	rate := s.settingService.GetDebugRequestLogSampleRate(ctx)
	if rate >= 100 {
		return true
	}
	return rand.IntN(100) < rate
}

// AcquireBuildSlot 在 BuildEntry 之前调用，限制并发解析数。
// 非阻塞：满了直接返回 false，调用方丢弃这条日志。
func (s *RequestDebugLogService) AcquireBuildSlot() bool {
	select {
	case s.buildSem <- struct{}{}:
		return true
	default:
		return false
	}
}

// ReleaseBuildSlot 在 BuildEntry 完成后释放（无论成功失败）。
func (s *RequestDebugLogService) ReleaseBuildSlot() {
	<-s.buildSem
}

// Enqueue 投入队列；队列槽位或字节配额满则丢弃，不阻塞主流程。
// F2 反压：除了 channel 长度限制（debugLogQueueSize 条），再加字节配额
// （debugLogQueueMaxBytes 字节），封死最坏内存占用。
func (s *RequestDebugLogService) Enqueue(log *RequestDebugLog) {
	if log == nil {
		return
	}
	size := int64(log.estimateSize())
	if s.queueBytes.Load()+size > debugLogQueueMaxBytes {
		slog.Warn("debug_log_queue_bytes_full_dropped",
			"request_id", log.RequestID, "size", size,
			"queue_bytes", s.queueBytes.Load())
		return
	}
	select {
	case s.queue <- log:
		s.queueBytes.Add(size)
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

// smartTruncateMinBytes 小于该阈值的 body 直接原样存，不进 SmartTruncate。
// 依据：Claude Code 首轮请求（system + tools schema）通常 30-80KB，
// 32KB 阈值能跳过非首轮的纯文本对话（绝大多数），又能保证带工具定义的
// 大请求走截断路径。这一招对总 CPU 占用的下降比换 JSON 库还猛——绝大多数
// 请求根本不进 SmartTruncate。
const smartTruncateMinBytes = 32 * 1024

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

	// 小 body 早返回：跳过 SmartTruncate 的 Unmarshal/遍历/Marshal 三步走。
	// JSON 校验在 PG JSONB 列入库时也会做，但这里先在应用层校验避免无谓的入库失败。
	if len(body) < smartTruncateMinBytes {
		if json.Valid(body) {
			assign(json.RawMessage(body), "")
			return
		}
		// 非 JSON 走文本兜底
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
	batchBytes := 0
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
		batchBytes = 0
	}

	for {
		select {
		case log, ok := <-s.queue:
			if !ok {
				flush()
				return
			}
			size := log.estimateSize()
			s.queueBytes.Add(-int64(size))
			batch = append(batch, log)
			batchBytes += size
			// 条数或字节任一达到上限即 flush，避免 50×1MB=50MB 巨型事务
			if len(batch) >= debugLogBatchSize || batchBytes >= debugLogBatchBytes {
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
					size := log.estimateSize()
					s.queueBytes.Add(-int64(size))
					batch = append(batch, log)
					batchBytes += size
					if batchBytes >= debugLogBatchBytes {
						flush()
					}
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

