package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// wrapUsageTaskWithCtx 把请求作用域的 ctxkey.RequestID / ClientRequestID 从
// parent ctx 提取并注入到 worker pool 重建的后台 ctx 中。
//
// 背景：UsageRecordWorkerPool.execute 自己 context.Background() 重建 ctx，
// 会丢弃 middleware.RequestLogger 注入的 RequestID。usage_logs 和 debug_log
// 都依赖该 key 解析 request_id，必须显式透传，否则两表 ID 永远错配。
func wrapUsageTaskWithCtx(parentCtx context.Context, task service.UsageRecordTask) service.UsageRecordTask {
	if parentCtx == nil || task == nil {
		return task
	}
	reqID, _ := parentCtx.Value(ctxkey.RequestID).(string)
	clientReqID, _ := parentCtx.Value(ctxkey.ClientRequestID).(string)
	if reqID == "" && clientReqID == "" {
		return task
	}
	return func(ctx context.Context) {
		if reqID != "" {
			ctx = context.WithValue(ctx, ctxkey.RequestID, reqID)
		}
		if clientReqID != "" {
			ctx = context.WithValue(ctx, ctxkey.ClientRequestID, clientReqID)
		}
		task(ctx)
	}
}
