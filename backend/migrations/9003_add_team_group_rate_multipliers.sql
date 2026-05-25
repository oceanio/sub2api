-- Per-team overrides for (group) rate_multiplier and rpm_override.
-- Mirrors user_group_rate_multipliers (migrations 047 + 127) but keyed on
-- team_id instead of user_id. Both columns are NULL-able so a single row can
-- carry just one override; row is hard-deleted by the sync layer when both
-- columns end up NULL.
--
-- Resolution priority (read by gateway / billing_cache_service):
--   rate_multiplier:  user_group  > team_group  > group.rate_multiplier  > system default
--   rpm_override   :  user_group  > team_group  > group.rpm_limit        > user.rpm_limit (ceiling)
-- team_group only applies when api_keys.team_id IS NOT NULL on the request.

CREATE TABLE IF NOT EXISTS team_group_rate_multipliers (
    id              BIGSERIAL     PRIMARY KEY,
    team_id         BIGINT        NOT NULL REFERENCES teams(id),
    group_id        BIGINT        NOT NULL REFERENCES groups(id),
    rate_multiplier DECIMAL(10,4) NULL,
    rpm_override    INTEGER       NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS team_group_rate_team_group_uniq_idx
    ON team_group_rate_multipliers(team_id, group_id);
CREATE INDEX IF NOT EXISTS team_group_rate_team_id_idx
    ON team_group_rate_multipliers(team_id);
CREATE INDEX IF NOT EXISTS team_group_rate_group_id_idx
    ON team_group_rate_multipliers(group_id);

COMMENT ON COLUMN team_group_rate_multipliers.rate_multiplier IS '团队在该分组的专属计费倍率；NULL 表示不覆盖（沿用 user_group / group / system 链）。';
COMMENT ON COLUMN team_group_rate_multipliers.rpm_override    IS '团队成员在该分组的 RPM 上限；NULL 表示沿用 group.rpm_limit；0 表示成员在该分组免 RPM 检查。';
