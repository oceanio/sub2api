// Fork: 系统日志 CSV 导出 — 复用 list 的 where + scan，但不分页，直接 LIMIT 取最近 N 条。
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ListSystemLogsForExport 返回最多 limit 条最近的系统日志和命中筛选条件的总数（用于 truncate 提示）。
// 不做分页；调用方负责把 limit clamp 到合理范围。返回的 logs 按 created_at DESC, id DESC 排序。
func (r *opsRepository) ListSystemLogsForExport(ctx context.Context, filter *service.OpsSystemLogFilter, limit int) ([]*service.OpsSystemLog, int, error) {
	if r == nil || r.db == nil {
		return nil, 0, sql.ErrConnDone
	}
	if filter == nil {
		filter = &service.OpsSystemLogFilter{}
	}
	if limit <= 0 {
		return []*service.OpsSystemLog{}, 0, nil
	}

	where, args, _ := buildOpsSystemLogsWhere(filter)

	countSQL := "SELECT COUNT(*) FROM ops_system_logs l " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
SELECT
  l.id,
  l.created_at,
  l.level,
  COALESCE(l.component, ''),
  COALESCE(l.message, ''),
  COALESCE(l.request_id, ''),
  COALESCE(l.client_request_id, ''),
  l.user_id,
  l.account_id,
  COALESCE(l.platform, ''),
  COALESCE(l.model, ''),
  COALESCE(l.extra::text, '{}')
FROM ops_system_logs l
` + where + `
ORDER BY l.created_at DESC, l.id DESC
LIMIT $` + itoa(len(args)+1)
	argsWithLimit := append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, argsWithLimit...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	logs := make([]*service.OpsSystemLog, 0, limit)
	for rows.Next() {
		item := &service.OpsSystemLog{}
		var userID sql.NullInt64
		var accountID sql.NullInt64
		var extraRaw string
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.Level,
			&item.Component,
			&item.Message,
			&item.RequestID,
			&item.ClientRequestID,
			&userID,
			&accountID,
			&item.Platform,
			&item.Model,
			&extraRaw,
		); err != nil {
			return nil, 0, err
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if accountID.Valid {
			v := accountID.Int64
			item.AccountID = &v
		}
		extraRaw = strings.TrimSpace(extraRaw)
		if extraRaw != "" && extraRaw != "null" && extraRaw != "{}" {
			extra := make(map[string]any)
			if err := json.Unmarshal([]byte(extraRaw), &extra); err == nil {
				item.Extra = extra
			}
		}
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
