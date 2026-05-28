package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type userGroupRateResolver struct {
	repo         UserGroupRateRepository
	cache        *gocache.Cache
	cacheTTL     time.Duration
	sf           *singleflight.Group
	logComponent string
}

func newUserGroupRateResolver(repo UserGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *userGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
	}
	if cache == nil {
		cache = gocache.New(cacheTTL, time.Minute)
	}
	if logComponent == "" {
		logComponent = "service.gateway"
	}
	if sf == nil {
		sf = &singleflight.Group{}
	}

	return &userGroupRateResolver{
		repo:         repo,
		cache:        cache,
		cacheTTL:     cacheTTL,
		sf:           sf,
		logComponent: logComponent,
	}
}

// ResolveOverride returns the user-group rate_multiplier override if present
// (nil = no override). Used by both Resolve (which falls back to groupDefault)
// and the multi-tier resolver chain (which falls through to the team-group
// resolver and then group default when this returns nil).
//
// Cache value type is *float64 so a "no override" outcome can be memoised
// (cached as nil) and distinguished from "haven't checked yet".
func (r *userGroupRateResolver) ResolveOverride(ctx context.Context, userID, groupID int64) *float64 {
	if r == nil || userID <= 0 || groupID <= 0 {
		return nil
	}

	key := fmt.Sprintf("%d:%d", userID, groupID)
	if r.cache != nil {
		if cached, ok := r.cache.Get(key); ok {
			if cached == nil {
				userGroupRateCacheHitTotal.Add(1)
				return nil
			}
			if ptr, castOK := cached.(*float64); castOK {
				userGroupRateCacheHitTotal.Add(1)
				return ptr
			}
			// Bad cache entry (legacy or wrong type) — fall through to reload,
			// not counted as a hit (preserves original metric semantics).
		}
	}
	if r.repo == nil {
		return nil
	}
	userGroupRateCacheMissTotal.Add(1)

	value, err, shared := r.sf.Do(key, func() (any, error) {
		if r.cache != nil {
			if cached, ok := r.cache.Get(key); ok {
				if cached == nil {
					userGroupRateCacheHitTotal.Add(1)
					return (*float64)(nil), nil
				}
				if ptr, castOK := cached.(*float64); castOK {
					userGroupRateCacheHitTotal.Add(1)
					return ptr, nil
				}
			}
		}

		userGroupRateCacheLoadTotal.Add(1)
		userRate, repoErr := r.repo.GetByUserAndGroup(ctx, userID, groupID)
		if repoErr != nil {
			return nil, repoErr
		}
		if r.cache != nil {
			r.cache.Set(key, userRate, r.cacheTTL)
		}
		return userRate, nil
	})
	if shared {
		userGroupRateCacheSFSharedTotal.Add(1)
	}
	if err != nil {
		userGroupRateCacheFallbackTotal.Add(1)
		logger.LegacyPrintf(r.logComponent, "get user group rate failed, fallback to no-override: user=%d group=%d err=%v", userID, groupID, err)
		return nil
	}

	if value == nil {
		return nil
	}
	ptr, ok := value.(*float64)
	if !ok {
		userGroupRateCacheFallbackTotal.Add(1)
		return nil
	}
	return ptr
}

// Resolve is the legacy "give me the rate multiplier with groupDefault fallback"
// API. Implemented as a thin wrapper over ResolveOverride.
func (r *userGroupRateResolver) Resolve(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if override := r.ResolveOverride(ctx, userID, groupID); override != nil {
		return *override
	}
	return groupDefaultMultiplier
}
