package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

// teamGroupRateResolver mirrors userGroupRateResolver but is keyed on
// (team_id, group_id). It only exposes ResolveOverride because team is
// never a leaf in the resolution chain — it always falls through to
// group default if absent, so the "with default" convenience method on
// the user-side resolver is not needed here.
type teamGroupRateResolver struct {
	repo         TeamGroupRateRepository
	cache        *gocache.Cache
	cacheTTL     time.Duration
	sf           *singleflight.Group
	logComponent string
}

func newTeamGroupRateResolver(repo TeamGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *teamGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL // 复用同一个常量
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
	return &teamGroupRateResolver{
		repo:         repo,
		cache:        cache,
		cacheTTL:     cacheTTL,
		sf:           sf,
		logComponent: logComponent,
	}
}

// ResolveOverride returns the team-group rate_multiplier override (nil = no
// override). Caller is responsible for falling through to group default.
func (r *teamGroupRateResolver) ResolveOverride(ctx context.Context, teamID, groupID int64) *float64 {
	if r == nil || teamID <= 0 || groupID <= 0 {
		return nil
	}

	key := fmt.Sprintf("%d:%d", teamID, groupID)
	if r.cache != nil {
		if cached, ok := r.cache.Get(key); ok {
			if cached == nil {
				teamGroupRateCacheHitTotal.Add(1)
				return nil
			}
			if ptr, castOK := cached.(*float64); castOK {
				teamGroupRateCacheHitTotal.Add(1)
				return ptr
			}
		}
	}
	if r.repo == nil {
		return nil
	}
	teamGroupRateCacheMissTotal.Add(1)

	value, err, shared := r.sf.Do(key, func() (any, error) {
		if r.cache != nil {
			if cached, ok := r.cache.Get(key); ok {
				if cached == nil {
					teamGroupRateCacheHitTotal.Add(1)
					return (*float64)(nil), nil
				}
				if ptr, castOK := cached.(*float64); castOK {
					teamGroupRateCacheHitTotal.Add(1)
					return ptr, nil
				}
			}
		}

		teamGroupRateCacheLoadTotal.Add(1)
		teamRate, repoErr := r.repo.GetByTeamAndGroup(ctx, teamID, groupID)
		if repoErr != nil {
			return nil, repoErr
		}
		if r.cache != nil {
			r.cache.Set(key, teamRate, r.cacheTTL)
		}
		return teamRate, nil
	})
	if shared {
		teamGroupRateCacheSFSharedTotal.Add(1)
	}
	if err != nil {
		teamGroupRateCacheFallbackTotal.Add(1)
		logger.LegacyPrintf(r.logComponent, "get team group rate failed, fallback to no-override: team=%d group=%d err=%v", teamID, groupID, err)
		return nil
	}

	if value == nil {
		return nil
	}
	ptr, ok := value.(*float64)
	if !ok {
		teamGroupRateCacheFallbackTotal.Add(1)
		return nil
	}
	return ptr
}
