-- Per-team switch: when FALSE, team_admin loses access to the Subscriptions
-- tab in the team management UI and the corresponding team_admin endpoints
-- return 403. Sys admin is unaffected and can still operate via /admin/teams.
--
-- DEFAULT TRUE for backward compatibility with existing teams. In PG 11+ this
-- ADD COLUMN ... DEFAULT is metadata-only (no full-table rewrite), so safe to
-- run online.

ALTER TABLE teams
    ADD COLUMN IF NOT EXISTS subscriptions_enabled BOOLEAN NOT NULL DEFAULT TRUE;
