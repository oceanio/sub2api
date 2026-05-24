-- Team management (v2): teams, team_members, team_admins, team_balance_logs
-- plus team_id columns on api_keys and user_subscriptions.
--
-- v2 model: membership (team_members) and admin role (team_admins) are split.
-- A user can be a paying member of at most ONE team but admin of MANY teams.

-- ── teams ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS teams (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100)  NOT NULL,
    balance         DECIMAL(20,8) NOT NULL DEFAULT 0,
    available_tags  JSONB,
    max_members     INT           NOT NULL DEFAULT 0,  -- 0 = unlimited; only sys admin can set
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS teams_name_idx       ON teams(name);
CREATE INDEX IF NOT EXISTS teams_deleted_at_idx ON teams(deleted_at);

-- ── team_members (paying members) ────────────────────────────────────────────
-- v2: role column removed. Admin role is now in team_admins.
CREATE TABLE IF NOT EXISTS team_members (
    id             BIGSERIAL PRIMARY KEY,
    team_id        BIGINT        NOT NULL REFERENCES teams(id),
    user_id        BIGINT        NOT NULL REFERENCES users(id),
    sub_quota      DECIMAL(20,8) NOT NULL DEFAULT 0,
    sub_quota_used DECIMAL(20,8) NOT NULL DEFAULT 0,
    tags           JSONB,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS team_members_team_id_idx    ON team_members(team_id);
CREATE INDEX IF NOT EXISTS team_members_user_id_idx    ON team_members(user_id);
CREATE INDEX IF NOT EXISTS team_members_deleted_at_idx ON team_members(deleted_at);

-- A user can only be a paying member of one team at a time.
CREATE UNIQUE INDEX IF NOT EXISTS team_members_user_unique_idx
    ON team_members(user_id) WHERE deleted_at IS NULL;
-- No duplicate (team, user) pairs in active records.
CREATE UNIQUE INDEX IF NOT EXISTS team_members_team_user_unique_idx
    ON team_members(team_id, user_id) WHERE deleted_at IS NULL;

-- ── team_admins (admin role) ────────────────────────────────────────────────
-- A user can be admin of MANY teams; a team can have MANY admins.
CREATE TABLE IF NOT EXISTS team_admins (
    id          BIGSERIAL PRIMARY KEY,
    team_id     BIGINT      NOT NULL REFERENCES teams(id),
    user_id     BIGINT      NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS team_admins_team_id_idx    ON team_admins(team_id);
CREATE INDEX IF NOT EXISTS team_admins_user_id_idx    ON team_admins(user_id);
CREATE INDEX IF NOT EXISTS team_admins_deleted_at_idx ON team_admins(deleted_at);

-- Each (team, user) pair has at most one active admin record.
CREATE UNIQUE INDEX IF NOT EXISTS team_admins_team_user_unique_idx
    ON team_admins(team_id, user_id) WHERE deleted_at IS NULL;

-- ── team_balance_logs ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS team_balance_logs (
    id             BIGSERIAL PRIMARY KEY,
    team_id        BIGINT        NOT NULL REFERENCES teams(id),
    type           VARCHAR(30)   NOT NULL,
    amount         DECIMAL(20,8) NOT NULL,
    operator_id    BIGINT        NOT NULL REFERENCES users(id),
    target_user_id BIGINT,
    note           TEXT,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS team_balance_logs_team_id_idx         ON team_balance_logs(team_id);
CREATE INDEX IF NOT EXISTS team_balance_logs_team_created_at_idx ON team_balance_logs(team_id, created_at);
CREATE INDEX IF NOT EXISTS team_balance_logs_operator_id_idx     ON team_balance_logs(operator_id);

-- ── api_keys: add team_id ─────────────────────────────────────────────────────
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS team_id BIGINT REFERENCES teams(id);
CREATE INDEX IF NOT EXISTS api_keys_team_id_idx ON api_keys(team_id);

-- ── user_subscriptions: add team_id ──────────────────────────────────────────
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS team_id BIGINT REFERENCES teams(id);
CREATE INDEX IF NOT EXISTS user_subscriptions_team_id_idx ON user_subscriptions(team_id);
