package service

import "context"

// TeamGroupRateEntry 是一条 team_group_rate_multipliers 记录的服务层投影，
// 用于 admin "团队×分组" 矩阵的展示与批量同步。Both rate_multiplier 和
// rpm_override 都是 *T —— nil 表示「该列没有覆盖」。Sync API 也用同样的
// nil 语义：值为 nil 表示清空该列；两列都被清空时整行 hard-delete。
type TeamGroupRateEntry struct {
	GroupID        int64
	RateMultiplier *float64
	RPMOverride    *int
}

// TeamGroupRateRepository 是 team_group_rate_multipliers 的仓储接口。
// 形状刻意与 UserGroupRateRepository 平行，只是把 user 维度换成 team 维度。
type TeamGroupRateRepository interface {
	// GetByTeamAndGroup 返回团队在指定分组的 rate_multiplier；nil 表示无覆盖。
	GetByTeamAndGroup(ctx context.Context, teamID, groupID int64) (*float64, error)

	// GetRPMOverrideByTeamAndGroup 返回团队在指定分组的 rpm_override；nil 表示无覆盖。
	GetRPMOverrideByTeamAndGroup(ctx context.Context, teamID, groupID int64) (*int, error)

	// GetByTeamID 一次性返回团队所有覆盖（rate 或 rpm 任一非 NULL）。
	// 用于 admin GET /admin/teams/:id 响应体一次性拼装两个 map。
	GetByTeamID(ctx context.Context, teamID int64) ([]TeamGroupRateEntry, error)

	// SyncTeamGroupRates 同步团队的 rate_multiplier 部分：
	//   - 值非 nil → upsert rate_multiplier（保留 rpm_override）
	//   - 值为 nil → 清空 rate_multiplier（保留 rpm_override）
	//   - rate_multiplier 与 rpm_override 都为 NULL 时整行 DELETE
	// 缺席的 group_id 不动。
	SyncTeamGroupRates(ctx context.Context, teamID int64, rates map[int64]*float64) error

	// SyncTeamGroupRPMOverrides 同步团队的 rpm_override 部分（语义同上）。
	SyncTeamGroupRPMOverrides(ctx context.Context, teamID int64, overrides map[int64]*int) error

	// DeleteByTeamID 清空团队所有覆盖（删团队时调用）。
	DeleteByTeamID(ctx context.Context, teamID int64) error

	// DeleteByGroupID 清空指定分组的所有团队覆盖（删分组时调用）。
	DeleteByGroupID(ctx context.Context, groupID int64) error
}
