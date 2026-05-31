// Fork: 系统日志 CSV 导出 service 层 — clamp limit + 调 repo + 计算 truncated。
package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpsSystemLogExportMaxRows 单次导出最大行数。超出此值的查询会被截断，UI 通过 X-Export-Truncated 提示用户。
const OpsSystemLogExportMaxRows = 50000

// ListSystemLogsForExport 取最近最多 limit 条系统日志用于 CSV 导出。
// 返回 (logs, totalMatched, truncated, err)，其中 truncated=true 表示命中数超过 limit、只返回了最近 limit 条。
func (s *OpsService) ListSystemLogsForExport(ctx context.Context, filter *OpsSystemLogFilter, limit int) ([]*OpsSystemLog, int, bool, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, 0, false, err
	}
	if s.opsRepo == nil {
		return []*OpsSystemLog{}, 0, false, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if limit <= 0 || limit > OpsSystemLogExportMaxRows {
		limit = OpsSystemLogExportMaxRows
	}
	if filter == nil {
		filter = &OpsSystemLogFilter{}
	}

	logs, total, err := s.opsRepo.ListSystemLogsForExport(ctx, filter, limit)
	if err != nil {
		return nil, 0, false, infraerrors.InternalServer("OPS_SYSTEM_LOG_EXPORT_FAILED", "Failed to export system logs").WithCause(err)
	}
	truncated := total > len(logs)
	return logs, total, truncated, nil
}
