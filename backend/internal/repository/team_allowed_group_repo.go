package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type teamAllowedGroupRepository struct {
	sql sqlExecutor
}

func NewTeamAllowedGroupRepository(sqlDB *sql.DB) service.TeamAllowedGroupRepository {
	return &teamAllowedGroupRepository{sql: sqlDB}
}

func (r *teamAllowedGroupRepository) ListByTeamID(ctx context.Context, teamID int64) ([]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT group_id FROM team_allowed_groups WHERE team_id = $1`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *teamAllowedGroupRepository) Add(ctx context.Context, teamID, groupID int64) error {
	_, err := r.sql.ExecContext(ctx,
		`INSERT INTO team_allowed_groups (team_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		teamID, groupID,
	)
	return err
}

func (r *teamAllowedGroupRepository) Remove(ctx context.Context, teamID, groupID int64) error {
	_, err := r.sql.ExecContext(ctx,
		`DELETE FROM team_allowed_groups WHERE team_id = $1 AND group_id = $2`,
		teamID, groupID,
	)
	return err
}

func (r *teamAllowedGroupRepository) ExistsForTeam(ctx context.Context, teamID, groupID int64) (bool, error) {
	var count int
	err := scanSingleRow(ctx, r.sql,
		`SELECT COUNT(*) FROM team_allowed_groups WHERE team_id = $1 AND group_id = $2`,
		[]any{teamID, groupID}, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
