-- 非事务部分：仅 CONCURRENTLY 索引（不能与其它 DDL 混用）。
-- 配对的事务文件 9004_team_addons.sql 负责 CREATE TABLE。

-- team_members 活跃成员的部分索引：所有 team-scoped 用量查询都用
--   user_id IN (SELECT user_id FROM team_members WHERE team_id = $N AND deleted_at IS NULL)
-- 既有 (team_id, user_id) 索引不覆盖 deleted_at，会触发 heap fetch。
CREATE INDEX CONCURRENTLY IF NOT EXISTS teammember_team_id_user_id_active_idx
    ON team_members(team_id, user_id)
    WHERE deleted_at IS NULL;
