-- 调试日志:
--   - request_text: 与 response_text 对称,JSON 解析失败/字段级截断不可行时的兜底文本列
--   - truncation_info: 字段级截断元信息(被砍的字段路径、字节数、剥离的图片数等),
--     供前端展示"第 N 条 user.content[0].text 被砍 12345 字节"
ALTER TABLE request_debug_logs
    ADD COLUMN IF NOT EXISTS request_text     TEXT,
    ADD COLUMN IF NOT EXISTS truncation_info  JSONB;
