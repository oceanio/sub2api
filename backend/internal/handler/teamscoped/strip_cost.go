package teamscoped

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Team usage responses must never expose the cost breakdown — total_cost,
// per-input/output cost, account cost, rate multipliers. The headline fee
// (actual_cost) is what team users actually pay against the team balance and
// stays visible. This applies for both team_admin and sys_admin viewing
// /admin/teams/:id/usage/* — the surface decides, not the role.
//
// Stripping happens at the handler edge so the cache stores already-stripped
// payloads and any future caller of the same service method (e.g. an export
// or background job) can opt into the raw numbers without a flag.

func stripUsageLogCost(l *dto.UsageLog) {
	if l == nil {
		return
	}
	l.InputCost = 0
	l.OutputCost = 0
	l.CacheCreationCost = 0
	l.CacheReadCost = 0
	l.TotalCost = 0
	l.RateMultiplier = 0
}

func stripUsageLogsCost(logs []dto.UsageLog) {
	for i := range logs {
		stripUsageLogCost(&logs[i])
	}
}

func stripUsageStatsCost(s *usagestats.UsageStats) {
	if s == nil {
		return
	}
	s.TotalCost = 0
	s.TotalAccountCost = nil
}

func stripTrendCost(points []usagestats.TrendDataPoint) {
	for i := range points {
		points[i].Cost = 0
	}
}

func stripModelStatsCost(stats []usagestats.ModelStat) {
	for i := range stats {
		stats[i].Cost = 0
		stats[i].AccountCost = 0
	}
}

func stripGroupStatsCost(stats []usagestats.GroupStat) {
	for i := range stats {
		stats[i].Cost = 0
		stats[i].AccountCost = 0
	}
}

func stripEndpointStatsCost(stats []usagestats.EndpointStat) {
	for i := range stats {
		stats[i].Cost = 0
	}
}

func stripUserBreakdownCost(items []usagestats.UserBreakdownItem) {
	for i := range items {
		items[i].Cost = 0
		items[i].AccountCost = 0
	}
}
