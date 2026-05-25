-- Partial index for team membership lookups that filter deleted_at IS NULL.
-- All team-scoped usage queries use the subquery:
--   user_id IN (SELECT user_id FROM team_members WHERE team_id = $N AND deleted_at IS NULL)
-- The existing (team_id, user_id) index does not cover deleted_at, requiring a
-- heap fetch for every row. This partial index covers the active-only subset,
-- eliminating the heap fetch for membership resolution.

CREATE INDEX CONCURRENTLY IF NOT EXISTS teammember_team_id_user_id_active_idx
    ON team_members(team_id, user_id)
    WHERE deleted_at IS NULL;
