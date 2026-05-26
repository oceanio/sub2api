-- 事务部分：CREATE TABLE + 普通索引。
-- 配对的非事务文件 9004_team_addons_notx.sql 负责非事务索引。

-- team_allowed_groups: 团队级别的专属分组授权。
CREATE TABLE IF NOT EXISTS team_allowed_groups (
    team_id    BIGINT NOT NULL,
    group_id   BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, group_id)
);

CREATE INDEX IF NOT EXISTS team_allowed_groups_group_id_idx
    ON team_allowed_groups(group_id);
