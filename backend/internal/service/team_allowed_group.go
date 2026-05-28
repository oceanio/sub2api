package service

import "context"

// TeamAllowedGroupRepository manages team-level authorization for exclusive groups.
type TeamAllowedGroupRepository interface {
	// ListByTeamID returns all group_ids authorized for the team.
	ListByTeamID(ctx context.Context, teamID int64) ([]int64, error)
	// Add authorizes a group for the team (idempotent).
	Add(ctx context.Context, teamID, groupID int64) error
	// Remove revokes a group's authorization from the team.
	Remove(ctx context.Context, teamID, groupID int64) error
	// ExistsForTeam checks whether a specific group is authorized for a team.
	ExistsForTeam(ctx context.Context, teamID, groupID int64) (bool, error)
}
