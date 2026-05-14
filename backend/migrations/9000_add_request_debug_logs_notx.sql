CREATE INDEX CONCURRENTLY IF NOT EXISTS
    idx_request_debug_logs_request_id
    ON request_debug_logs (request_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS
    idx_request_debug_logs_user_created
    ON request_debug_logs (user_id, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS
    idx_request_debug_logs_expires_at
    ON request_debug_logs (expires_at);
