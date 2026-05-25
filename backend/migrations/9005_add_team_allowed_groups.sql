CREATE TABLE IF NOT EXISTS team_allowed_groups (
    team_id    BIGINT NOT NULL,
    group_id   BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, group_id)
);

CREATE INDEX IF NOT EXISTS team_allowed_groups_group_id_idx ON team_allowed_groups(group_id);
