package service

import "context"

// ResolveCorrelationID returns the request-scope identifier shared by usage_logs
// and request_debug_logs so the two tables can be JOINed on a single string.
// The caller must use ctx from c.Request.Context() so the middleware-injected
// RequestID is in scope; the upstream's x-request-id is only used as a
// fallback for callers that haven't propagated the gateway middleware ctx.
//
// Fork-only helper for the request-debug-log feature; kept here in its own
// file so gateway_service.go stays unchanged when upstream evolves the
// underlying resolveUsageBillingRequestID logic.
func ResolveCorrelationID(ctx context.Context, upstreamRequestID string) string {
	return resolveUsageBillingRequestID(ctx, upstreamRequestID)
}
