package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// teamGroupRateRepository 与 userGroupRateRepository 完全对称，只是把 user 维度
// 换成 team 维度。两列（rate_multiplier、rpm_override）均可 NULL，DELETE 行的条件是
// 两列都为 NULL 后整行删除。
type teamGroupRateRepository struct {
	sql sqlExecutor
}

// NewTeamGroupRateRepository 创建团队专属分组倍率/RPM 仓储。
func NewTeamGroupRateRepository(sqlDB *sql.DB) service.TeamGroupRateRepository {
	return &teamGroupRateRepository{sql: sqlDB}
}

func (r *teamGroupRateRepository) GetByTeamAndGroup(ctx context.Context, teamID, groupID int64) (*float64, error) {
	var rate sql.NullFloat64
	err := scanSingleRow(ctx, r.sql,
		`SELECT rate_multiplier FROM team_group_rate_multipliers WHERE team_id = $1 AND group_id = $2`,
		[]any{teamID, groupID}, &rate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rate.Valid {
		return nil, nil
	}
	v := rate.Float64
	return &v, nil
}

func (r *teamGroupRateRepository) GetRPMOverrideByTeamAndGroup(ctx context.Context, teamID, groupID int64) (*int, error) {
	var rpm sql.NullInt32
	err := scanSingleRow(ctx, r.sql,
		`SELECT rpm_override FROM team_group_rate_multipliers WHERE team_id = $1 AND group_id = $2`,
		[]any{teamID, groupID}, &rpm)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rpm.Valid {
		return nil, nil
	}
	v := int(rpm.Int32)
	return &v, nil
}

func (r *teamGroupRateRepository) GetByTeamID(ctx context.Context, teamID int64) ([]service.TeamGroupRateEntry, error) {
	rows, err := r.sql.QueryContext(ctx,
		`SELECT group_id, rate_multiplier, rpm_override
		 FROM team_group_rate_multipliers
		 WHERE team_id = $1`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []service.TeamGroupRateEntry
	for rows.Next() {
		var groupID int64
		var rate sql.NullFloat64
		var rpm sql.NullInt32
		if err := rows.Scan(&groupID, &rate, &rpm); err != nil {
			return nil, err
		}
		entry := service.TeamGroupRateEntry{GroupID: groupID}
		if rate.Valid {
			v := rate.Float64
			entry.RateMultiplier = &v
		}
		if rpm.Valid {
			v := int(rpm.Int32)
			entry.RPMOverride = &v
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// SyncTeamGroupRates —— 与 SyncUserGroupRates 对称：
//   - 值非 nil → upsert rate_multiplier，保留已有 rpm_override
//   - 值为 nil → 清空 rate_multiplier，保留 rpm_override；若 rpm_override 也为 NULL 则整行删除
//   - 不在 map 中的 group_id 不动
func (r *teamGroupRateRepository) SyncTeamGroupRates(ctx context.Context, teamID int64, rates map[int64]*float64) error {
	if len(rates) == 0 {
		return nil
	}

	var clearGroupIDs []int64
	upsertGroupIDs := make([]int64, 0, len(rates))
	upsertRates := make([]float64, 0, len(rates))
	for groupID, rate := range rates {
		if rate == nil {
			clearGroupIDs = append(clearGroupIDs, groupID)
		} else {
			upsertGroupIDs = append(upsertGroupIDs, groupID)
			upsertRates = append(upsertRates, *rate)
		}
	}

	if len(clearGroupIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE team_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE team_id = $1 AND group_id = ANY($2)
		`, teamID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx,
			`DELETE FROM team_group_rate_multipliers WHERE team_id = $1 AND group_id = ANY($2) AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			teamID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
	}

	if len(upsertGroupIDs) > 0 {
		now := time.Now()
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO team_group_rate_multipliers (team_id, group_id, rate_multiplier, created_at, updated_at)
			SELECT
				$1::bigint,
				data.group_id,
				data.rate_multiplier,
				$2::timestamptz,
				$2::timestamptz
			FROM unnest($3::bigint[], $4::double precision[]) AS data(group_id, rate_multiplier)
			ON CONFLICT (team_id, group_id)
			DO UPDATE SET
				rate_multiplier = EXCLUDED.rate_multiplier,
				updated_at = EXCLUDED.updated_at
		`, teamID, now, pq.Array(upsertGroupIDs), pq.Array(upsertRates))
		if err != nil {
			return err
		}
	}
	return nil
}

// SyncTeamGroupRPMOverrides —— 与 SyncTeamGroupRates 完全镜像，只是同步另一列。
func (r *teamGroupRateRepository) SyncTeamGroupRPMOverrides(ctx context.Context, teamID int64, overrides map[int64]*int) error {
	if len(overrides) == 0 {
		return nil
	}

	var clearGroupIDs []int64
	upsertGroupIDs := make([]int64, 0, len(overrides))
	upsertValues := make([]int32, 0, len(overrides))
	for groupID, rpm := range overrides {
		if rpm == nil {
			clearGroupIDs = append(clearGroupIDs, groupID)
		} else {
			upsertGroupIDs = append(upsertGroupIDs, groupID)
			upsertValues = append(upsertValues, int32(*rpm))
		}
	}

	if len(clearGroupIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE team_group_rate_multipliers
			SET rpm_override = NULL, updated_at = NOW()
			WHERE team_id = $1 AND group_id = ANY($2)
		`, teamID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx,
			`DELETE FROM team_group_rate_multipliers WHERE team_id = $1 AND group_id = ANY($2) AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			teamID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
	}

	if len(upsertGroupIDs) > 0 {
		now := time.Now()
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO team_group_rate_multipliers (team_id, group_id, rpm_override, created_at, updated_at)
			SELECT $1::bigint, data.group_id, data.rpm_override, $2::timestamptz, $2::timestamptz
			FROM unnest($3::bigint[], $4::integer[]) AS data(group_id, rpm_override)
			ON CONFLICT (team_id, group_id)
			DO UPDATE SET rpm_override = EXCLUDED.rpm_override, updated_at = EXCLUDED.updated_at
		`, teamID, now, pq.Array(upsertGroupIDs), pq.Array(upsertValues))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *teamGroupRateRepository) DeleteByTeamID(ctx context.Context, teamID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM team_group_rate_multipliers WHERE team_id = $1`, teamID)
	return err
}

func (r *teamGroupRateRepository) DeleteByGroupID(ctx context.Context, groupID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM team_group_rate_multipliers WHERE group_id = $1`, groupID)
	return err
}
