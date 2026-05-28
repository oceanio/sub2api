package service

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Fork addition: request-debug-log feature settings (5 keys behind a 60s
// in-memory cache to avoid hammering the settings table from the gateway
// hot path). Lives in its own file so upstream-tracking sees setting_service.go
// with minimal touches when origin/main churns the surrounding code.

type cachedDebugLogSettings struct {
	enabled       bool
	ttl           time.Duration
	sampleRate    int
	redactHeaders bool
	bodyLimit     int
	expiresAt     int64 // unix nano
}

var debugLogSettingsCache atomic.Value // *cachedDebugLogSettings

const (
	debugLogSettingsCacheTTL = 60 * time.Second
	debugLogSettingsErrorTTL = 5 * time.Second

	debugLogDefaultTTLHours = 168 // 7 days
	// Default sample rate dropped from 100 to 10: debug logs are a
	// troubleshooting tool, not an audit log. At 100% the high-QPS gateway
	// pays meaningful CPU/GC/DB overhead (see RequestDebugLogService comment).
	// Ops can dial it back up via settings when investigating.
	debugLogDefaultSampleRate = 10
	debugLogDefaultBodyLimit  = 1024
)

// loadDebugLogSettings reads (or refreshes) the cached debug-log config.
// Collapses 5 DB lookups per gateway request into one batched lookup every 60s.
func (s *SettingService) loadDebugLogSettings(ctx context.Context) *cachedDebugLogSettings {
	if cached, ok := debugLogSettingsCache.Load().(*cachedDebugLogSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached
		}
	}

	keys := []string{
		SettingKeyDebugRequestLogEnabled,
		SettingKeyDebugRequestLogTTLHours,
		SettingKeyDebugRequestLogSampleRate,
		SettingKeyDebugRequestLogRedactHeaders,
		SettingKeyDebugRequestLogBodyLimit,
	}
	values, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		// On read failure cache the "disabled" state with a short TTL to
		// avoid hammering the table during outages.
		fallback := &cachedDebugLogSettings{
			enabled:       false,
			ttl:           debugLogDefaultTTLHours * time.Hour,
			sampleRate:    debugLogDefaultSampleRate,
			redactHeaders: true,
			bodyLimit:     debugLogDefaultBodyLimit,
			expiresAt:     time.Now().Add(debugLogSettingsErrorTTL).UnixNano(),
		}
		debugLogSettingsCache.Store(fallback)
		return fallback
	}

	settings := &cachedDebugLogSettings{
		enabled:       values[SettingKeyDebugRequestLogEnabled] == "true",
		ttl:           debugLogDefaultTTLHours * time.Hour,
		sampleRate:    debugLogDefaultSampleRate,
		redactHeaders: values[SettingKeyDebugRequestLogRedactHeaders] != "false",
		bodyLimit:     debugLogDefaultBodyLimit,
		expiresAt:     time.Now().Add(debugLogSettingsCacheTTL).UnixNano(),
	}
	if v, errp := strconv.Atoi(strings.TrimSpace(values[SettingKeyDebugRequestLogTTLHours])); errp == nil && v > 0 {
		settings.ttl = time.Duration(v) * time.Hour
	}
	if v, errp := strconv.Atoi(strings.TrimSpace(values[SettingKeyDebugRequestLogSampleRate])); errp == nil && v >= 1 && v <= 100 {
		settings.sampleRate = v
	}
	if v, errp := strconv.Atoi(strings.TrimSpace(values[SettingKeyDebugRequestLogBodyLimit])); errp == nil && v >= 0 {
		settings.bodyLimit = v
	}
	debugLogSettingsCache.Store(settings)
	return settings
}

func (s *SettingService) IsDebugRequestLogEnabled(ctx context.Context) bool {
	return s.loadDebugLogSettings(ctx).enabled
}

func (s *SettingService) GetDebugRequestLogTTL(ctx context.Context) time.Duration {
	return s.loadDebugLogSettings(ctx).ttl
}

func (s *SettingService) GetDebugRequestLogSampleRate(ctx context.Context) int {
	return s.loadDebugLogSettings(ctx).sampleRate
}

func (s *SettingService) IsDebugRequestLogRedactHeaders(ctx context.Context) bool {
	return s.loadDebugLogSettings(ctx).redactHeaders
}

func (s *SettingService) GetDebugRequestLogBodyLimit(ctx context.Context) int {
	return s.loadDebugLogSettings(ctx).bodyLimit
}

// applyDebugLogParse populates the debug-log fields on a SystemSettings from
// the raw settings map. Called from parseSettings via a one-line hook.
func applyDebugLogParse(result *SystemSettings, settings map[string]string) {
	result.DebugRequestLogEnabled = settings[SettingKeyDebugRequestLogEnabled] == "true"
	if v, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyDebugRequestLogTTLHours])); err == nil && v > 0 {
		result.DebugRequestLogTTLHours = v
	} else {
		result.DebugRequestLogTTLHours = debugLogDefaultTTLHours
	}
	if v, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyDebugRequestLogSampleRate])); err == nil && v >= 1 && v <= 100 {
		result.DebugRequestLogSampleRate = v
	} else {
		result.DebugRequestLogSampleRate = debugLogDefaultSampleRate
	}
	result.DebugRequestLogRedactHeaders = settings[SettingKeyDebugRequestLogRedactHeaders] != "false"
	if v, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyDebugRequestLogBodyLimit])); err == nil && v >= 0 {
		result.DebugRequestLogBodyLimit = v
	} else {
		result.DebugRequestLogBodyLimit = debugLogDefaultBodyLimit
	}
}

// applyDebugLogUpdates writes the debug-log fields from SystemSettings into
// the updates map. Called from buildSystemSettingsUpdates via a one-line hook.
func applyDebugLogUpdates(updates map[string]string, settings *SystemSettings) {
	updates[SettingKeyDebugRequestLogEnabled] = strconv.FormatBool(settings.DebugRequestLogEnabled)
	updates[SettingKeyDebugRequestLogTTLHours] = strconv.Itoa(settings.DebugRequestLogTTLHours)
	updates[SettingKeyDebugRequestLogSampleRate] = strconv.Itoa(settings.DebugRequestLogSampleRate)
	updates[SettingKeyDebugRequestLogRedactHeaders] = strconv.FormatBool(settings.DebugRequestLogRedactHeaders)
	updates[SettingKeyDebugRequestLogBodyLimit] = strconv.Itoa(settings.DebugRequestLogBodyLimit)
}

// applyDebugLogCacheRefresh warms the debug-log cache from a freshly-loaded
// SystemSettings. Called from refreshCachedSettings via a one-line hook.
func applyDebugLogCacheRefresh(settings *SystemSettings) {
	debugLogSettingsCache.Store(&cachedDebugLogSettings{
		enabled:       settings.DebugRequestLogEnabled,
		ttl:           time.Duration(settings.DebugRequestLogTTLHours) * time.Hour,
		sampleRate:    settings.DebugRequestLogSampleRate,
		redactHeaders: settings.DebugRequestLogRedactHeaders,
		bodyLimit:     settings.DebugRequestLogBodyLimit,
		expiresAt:     time.Now().Add(debugLogSettingsCacheTTL).UnixNano(),
	})
}
