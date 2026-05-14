CREATE TABLE IF NOT EXISTS request_debug_logs (
    id              BIGSERIAL PRIMARY KEY,
    request_id      VARCHAR(64)  NOT NULL,
    user_id         BIGINT,
    api_key_id      BIGINT,
    group_id        BIGINT,
    model           VARCHAR(200),
    stream          BOOLEAN      NOT NULL DEFAULT FALSE,
    request_headers JSONB,
    request_body    JSONB,
    response_body   JSONB,
    response_text   TEXT,
    truncated       BOOLEAN      NOT NULL DEFAULT FALSE,
    body_bytes      INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ  NOT NULL
);
