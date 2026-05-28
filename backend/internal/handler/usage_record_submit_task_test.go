package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newUsageRecordTestPool(t *testing.T) *service.UsageRecordWorkerPool {
	t.Helper()
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	return pool
}

func TestGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &GatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &GatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
	})
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

// TestGatewayHandlerSubmitUsageRecordTask_PropagatesRequestID 验证 parent ctx
// 携带的 ctxkey.RequestID / ClientRequestID 能透传到 worker pool 重建后的 ctx。
// 这是修复 usage_logs.request_id 与 request_debug_logs.request_id 错配的关键。
func TestGatewayHandlerSubmitUsageRecordTask_PropagatesRequestID(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &GatewayHandler{usageRecordWorkerPool: pool}

	parent := context.WithValue(context.Background(), ctxkey.RequestID, "req-xyz")
	parent = context.WithValue(parent, ctxkey.ClientRequestID, "client-abc")

	type seen struct {
		req, client string
	}
	got := make(chan seen, 1)
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		r, _ := ctx.Value(ctxkey.RequestID).(string)
		c, _ := ctx.Value(ctxkey.ClientRequestID).(string)
		got <- seen{req: r, client: c}
	})

	select {
	case s := <-got:
		require.Equal(t, "req-xyz", s.req)
		require.Equal(t, "client-abc", s.client)
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
	})
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestOpenAIGatewayHandlerSubmitMandatoryUsageRecordTask_DroppedTaskSyncFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitMandatoryUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "mandatory usage task must run synchronously when async submit is dropped")
}

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_ImageResultUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{ImageCount: 1}, func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "image usage task must be mandatory when async submit is dropped")
}
