-- 调试日志主表:异步采样写入,定期清理(详见 RequestDebugLogService)。
-- 列说明:
--   - request_headers / response_headers: 经脱敏后的 HTTP 头(JSONB)
--   - request_body / response_body:       结构化字段级智能截断后的请求/响应体
--   - request_text / response_text:       字段级截断不可行/流式聚合失败时的原始文本兜底
--   - truncation_info:                    截断元信息(被砍字段路径、字节数、剥离的图片数等)
--   - body_bytes:                         原始请求体+响应体字节数(截断前)
--   - expires_at:                         到期清理时间
CREATE TABLE IF NOT EXISTS request_debug_logs (
    id               BIGSERIAL PRIMARY KEY,
    request_id       VARCHAR(64)  NOT NULL,
    user_id          BIGINT,
    api_key_id       BIGINT,
    group_id         BIGINT,
    model            VARCHAR(200),
    stream           BOOLEAN      NOT NULL DEFAULT FALSE,
    request_headers  JSONB,
    request_body     JSONB,
    request_text     TEXT,
    response_headers JSONB,
    response_body    JSONB,
    response_text    TEXT,
    truncated        BOOLEAN      NOT NULL DEFAULT FALSE,
    truncation_info  JSONB,
    body_bytes       INT          NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ  NOT NULL
);
