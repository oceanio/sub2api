package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

// Fork addition: count_tokens fallback to local tiktoken estimation when the
// upstream rejects the endpoint (Bedrock/Antigravity), returns 4xx/5xx, or has
// previously been marked unsupported. Account state is left untouched on
// errors so a flaky probe never trips rate limits / bans.
//
// Lives in its own file so upstream-tracking sees the original gateway_service
// methods (ForwardCountTokens + forwardCountTokensAnthropicAPIKeyPassthrough)
// as small modifications rather than 250-line rewrites — keeps merge surface
// focused when origin/main churns count_tokens code.
//
// The struct fields backing this feature (countTokensSupportCache,
// countTokensProbeSF) stay on GatewayService itself, since Go requires struct
// fields to live in the same file as the struct declaration.

// countTokensSupportEntry caches whether an upstream account supports the
// count_tokens endpoint, with an absolute expiry. Pointer values stored in the
// sync.Map so the swap is cheap.
type countTokensSupportEntry struct {
	supported bool
	expiresAt int64 // unix nano
}

const countTokensSupportCacheTTL = 300 * time.Second

// countTokensSupportStatus is the three-state result of a support lookup:
// Unknown (no entry / expired), Yes (upstream confirmed), No (upstream rejected).
type countTokensSupportStatus int

const (
	countTokensSupportUnknown countTokensSupportStatus = iota
	countTokensSupportYes
	countTokensSupportNo
)

// lookupCountTokensSupport reads the cache. Expired entries are evicted and
// reported as Unknown so the caller will re-probe.
func (s *GatewayService) lookupCountTokensSupport(accountID int64) countTokensSupportStatus {
	if accountID <= 0 {
		return countTokensSupportUnknown
	}
	v, ok := s.countTokensSupportCache.Load(accountID)
	if !ok {
		return countTokensSupportUnknown
	}
	entry, _ := v.(*countTokensSupportEntry)
	if entry == nil || time.Now().UnixNano() >= entry.expiresAt {
		s.countTokensSupportCache.Delete(accountID)
		return countTokensSupportUnknown
	}
	if entry.supported {
		return countTokensSupportYes
	}
	return countTokensSupportNo
}

// recordCountTokensSupport writes a cache entry with TTL = countTokensSupportCacheTTL.
func (s *GatewayService) recordCountTokensSupport(accountID int64, supported bool) {
	if accountID <= 0 {
		return
	}
	s.countTokensSupportCache.Store(accountID, &countTokensSupportEntry{
		supported: supported,
		expiresAt: time.Now().Add(countTokensSupportCacheTTL).UnixNano(),
	})
}

// countTokensProbeKey turns the account id into a singleflight key.
func countTokensProbeKey(accountID int64) string {
	return strconv.FormatInt(accountID, 10)
}

// forwardCountTokensWithFallback is the orchestration wrapped around the
// upstream dispatch: cache hit → fast path; cache miss → singleflight probe;
// follower → re-read cache after probe completes. ForwardCountTokens (in
// gateway_service.go) delegates here after validating the request body.
func (s *GatewayService) forwardCountTokensWithFallback(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) error {
	if account == nil {
		return s.forwardCountTokensDispatch(ctx, c, account, parsed)
	}

	switch s.lookupCountTokensSupport(account.ID) {
	case countTokensSupportNo:
		return s.localCountTokensEstimate(c, account, parsed, "upstream marked unsupported (cached)")
	case countTokensSupportYes:
		return s.forwardCountTokensDispatch(ctx, c, account, parsed)
	}

	// Unknown: singleflight coalesces concurrent callers. The first goroutine
	// probes upstream and writes the cache inside the closure; followers wait
	// here and then re-read the cache to decide their own path (each has its
	// own request body, so the response can't be shared).
	isProber := false
	_, sfErr, _ := s.countTokensProbeSF.Do(countTokensProbeKey(account.ID), func() (any, error) {
		isProber = true
		return nil, s.forwardCountTokensDispatch(ctx, c, account, parsed)
	})
	if isProber {
		return sfErr
	}

	// Follower path. If the prober failed with a network error it skipped the
	// cache write — we fall back to local estimation to prevent a thundering
	// herd hammering a downed upstream.
	switch s.lookupCountTokensSupport(account.ID) {
	case countTokensSupportYes:
		return s.forwardCountTokensDispatch(ctx, c, account, parsed)
	case countTokensSupportNo:
		return s.localCountTokensEstimate(c, account, parsed, "upstream marked unsupported (cached)")
	default:
		return s.localCountTokensEstimate(c, account, parsed, "probe inconclusive, falling back to local")
	}
}

// localCountTokensEstimate returns the local tiktoken estimate as a 200 OK,
// matching the upstream count_tokens response shape so clients can treat it
// transparently. Used on cache hit (No), upstream errors, and prober failure.
func (s *GatewayService) localCountTokensEstimate(c *gin.Context, account *Account, parsed *ParsedRequest, reason string) error {
	inputTokens := estimateInputTokens(parsed)
	if account != nil {
		logger.LegacyPrintf("service.gateway",
			"[count_tokens] %s, using local estimation: %d tokens (account=%d name=%s)",
			reason, inputTokens, account.ID, account.Name)
	}
	c.JSON(http.StatusOK, gin.H{"input_tokens": inputTokens})
	return nil
}

// forwardCountTokensDispatch is the real upstream call. Routes by account
// type (Anthropic API-key passthrough / Bedrock / Antigravity / standard
// count_tokens) and updates the support cache on terminal states.
func (s *GatewayService) forwardCountTokensDispatch(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) error {
	if account != nil && account.IsAnthropicAPIKeyPassthroughEnabled() {
		passthroughBody := parsed.Body
		if reqModel := parsed.Model; reqModel != "" {
			if mappedModel := account.GetMappedModel(reqModel); mappedModel != reqModel {
				passthroughBody = s.replaceModelInBody(passthroughBody, mappedModel)
				logger.LegacyPrintf("service.gateway", "CountTokens passthrough model mapping: %s -> %s (account: %s)", reqModel, mappedModel, account.Name)
			}
		}
		return s.forwardCountTokensAnthropicAPIKeyPassthrough(ctx, c, account, passthroughBody, parsed)
	}

	// Bedrock has no count_tokens endpoint — fall back immediately without probing.
	if account != nil && account.IsBedrock() {
		s.recordCountTokensSupport(account.ID, false)
		return s.localCountTokensEstimate(c, account, parsed, "Bedrock does not support count_tokens")
	}

	body := parsed.Body
	reqModel := parsed.Model

	body = StripEmptyTextBlocks(body)

	isClaudeCodeCT := IsClaudeCodeClient(ctx) || isClaudeCodeClient(c.GetHeader("User-Agent"), parsed.MetadataUserID)
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCodeCT

	if shouldMimicClaudeCode {
		normalizeOpts := claudeOAuthNormalizeOptions{stripSystemCacheControl: true}
		body, reqModel = normalizeClaudeOAuthRequestBody(body, reqModel, normalizeOpts)

		body = s.rewriteMessageCacheControlIfEnabled(ctx, body)
		if rw := buildToolNameRewriteFromBody(body); rw != nil {
			body = applyToolNameRewriteToBody(body, rw)
		} else {
			body = applyToolsLastCacheBreakpoint(body)
		}
	}

	// Antigravity also lacks count_tokens — same fallback as Bedrock.
	if account.Platform == PlatformAntigravity {
		s.recordCountTokensSupport(account.ID, false)
		return s.localCountTokensEstimate(c, account, parsed, "Antigravity does not support count_tokens")
	}

	if reqModel != "" {
		mappedModel := reqModel
		mappingSource := ""
		if account.Type == AccountTypeAPIKey {
			mappedModel = account.GetMappedModel(reqModel)
			if mappedModel != reqModel {
				mappingSource = "account"
			}
		}
		if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
			normalized := claude.NormalizeModelID(reqModel)
			if normalized != reqModel {
				mappedModel = normalized
				mappingSource = "prefix"
			}
		}
		if mappedModel != reqModel {
			body = s.replaceModelInBody(body, mappedModel)
			reqModel = mappedModel
			logger.LegacyPrintf("service.gateway", "CountTokens model mapping applied: %s -> %s (account: %s, source=%s)", parsed.Model, mappedModel, account.Name, mappingSource)
		}
	}

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}

	upstreamReq, err := s.buildCountTokensRequest(ctx, c, account, body, token, tokenType, reqModel, shouldMimicClaudeCode)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			proxyURL = account.Proxy.URL()
		}
	}

	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(err.Error()), "")
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}

	countTokensTooLarge := func(c *gin.Context) {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, countTokensTooLarge)
	_ = resp.Body.Close()
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		}
		return err
	}

	// Error responses: count_tokens is a light probe — never let it trip rate
	// limits or bans. Always fall back to local estimation and mark the
	// account as unsupported for the TTL window.
	if resp.StatusCode >= 400 {
		s.recordCountTokensSupport(account.ID, false)
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			logger.LegacyPrintf("service.gateway",
				"count_tokens upstream error %d (account=%d platform=%s type=%s) [isolated, account state unchanged]: %s",
				resp.StatusCode,
				account.ID,
				account.Platform,
				account.Type,
				truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
			)
		}
		inputTokens := estimateInputTokens(parsed)
		logger.LegacyPrintf("service.gateway",
			"[count_tokens] upstream error %d, falling back to local estimation: %d tokens (account=%d name=%s msg=%s)",
			resp.StatusCode, inputTokens, account.ID, account.Name, truncateString(upstreamMsg, 256))
		c.JSON(http.StatusOK, gin.H{"input_tokens": inputTokens})
		return nil
	}

	s.recordCountTokensSupport(account.ID, true)
	c.Data(resp.StatusCode, "application/json", respBody)
	return nil
}
