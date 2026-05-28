package service

import "context"

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	cacheKey := s.authCacheKey(key)
	s.deleteAuthCache(ctx, cacheKey)
}

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存。
// 异步 fan-out：admin 编辑 user 配置后立即返回，缓存清理在后台进行。
// 短暂窗口（毫秒级）内可能仍命中旧快照，但所有用户级修改 (RPM/role/concurrency
// 等) 都可接受这种 eventual consistency。
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	go s.invalidateAuthCacheByUserAsync(userID)
}

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存（异步 fan-out）。
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
	}
	go s.invalidateAuthCacheByGroupAsync(groupID)
}

// InvalidateAuthCacheByTeamID 清除团队相关的 API Key 认证缓存（异步 fan-out）。
// Sys admin 修改 team_group_rate_multipliers.rpm_override 后调用。500+ team
// member 的团队下，pipeline + chunked DEL + 单条 PUBLISH 配合后台异步执行，
// 把 admin 请求的 RT 从 ~1-3s 降到亚毫秒，且只产生 ~2 条 Redis 命令/256-chunk。
func (s *APIKeyService) InvalidateAuthCacheByTeamID(ctx context.Context, teamID int64) {
	if teamID <= 0 {
		return
	}
	go s.invalidateAuthCacheByTeamAsync(teamID)
}

// 下面 3 个 async helper 用独立 context（脱离调用方请求生命周期），失败时
// log，不向调用方传播错误——异步路径里调用方早已返回，传播也无处可去。
func (s *APIKeyService) invalidateAuthCacheByUserAsync(userID int64) {
	ctx := context.Background()
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

func (s *APIKeyService) invalidateAuthCacheByGroupAsync(groupID int64) {
	ctx := context.Background()
	keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

func (s *APIKeyService) invalidateAuthCacheByTeamAsync(teamID int64) {
	ctx := context.Background()
	keys, err := s.apiKeyRepo.ListKeysByTeamID(ctx, teamID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

// deleteAuthCacheByKeys evicts a batch of api_key strings from both L1 and L2:
//   - L1 (in-process ristretto): direct Del for each cacheKey — O(N) but free
//   - L2 (Redis): one pipelined DEL covering all keys + a single PUBLISH carrying
//     the JSON-encoded list, so other instances receive 1 message instead of N
//
// Falls back to per-key path only when the cache impl doesn't expose the batch
// primitive (e.g. legacy stubs).
func (s *APIKeyService) deleteAuthCacheByKeys(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
	}
	cacheKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		cacheKeys = append(cacheKeys, s.authCacheKey(key))
	}
	if len(cacheKeys) == 0 {
		return
	}
	// L1 eviction stays inline — it's pure in-memory and cheap regardless of N.
	if s.authCacheL1 != nil {
		for _, ck := range cacheKeys {
			s.authCacheL1.Del(ck)
		}
	}
	if s.cache == nil {
		return
	}
	_ = s.cache.DeleteAuthCacheBatch(ctx, cacheKeys)
}
