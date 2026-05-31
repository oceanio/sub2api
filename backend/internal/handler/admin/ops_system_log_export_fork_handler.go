// Fork: 系统日志 CSV 导出 handler — 复用 ListSystemLogs 的筛选解析，调 service 后输出 CSV。
package admin

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ExportSystemLogs streams the indexed system logs matching the current filter as CSV.
// GET /api/v1/admin/ops/system-logs/export
// Query params: same as ListSystemLogs, plus optional `limit` (default+max = OpsSystemLogExportMaxRows).
// 响应头：
//   - Content-Type: text/csv; charset=utf-8
//   - Content-Disposition: attachment; filename=ops-system-logs-<ts>.csv
//   - X-Export-Total: 命中筛选条件的总行数（未截断前）
//   - X-Export-Truncated: "true" 时表示导出已截断到 limit 行
func (h *OpsHandler) ExportSystemLogs(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	start, end, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	filter := &service.OpsSystemLogFilter{
		StartTime:       &start,
		EndTime:         &end,
		Level:           strings.TrimSpace(c.Query("level")),
		Component:       strings.TrimSpace(c.Query("component")),
		RequestID:       strings.TrimSpace(c.Query("request_id")),
		ClientRequestID: strings.TrimSpace(c.Query("client_request_id")),
		Platform:        strings.TrimSpace(c.Query("platform")),
		Model:           strings.TrimSpace(c.Query("model")),
		Query:           strings.TrimSpace(c.Query("q")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := strings.TrimSpace(c.Query("account_id")); v != "" {
		id, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		filter.AccountID = &id
	}

	limit := service.OpsSystemLogExportMaxRows
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		n, parseErr := strconv.Atoi(v)
		if parseErr != nil || n <= 0 {
			response.BadRequest(c, "Invalid limit")
			return
		}
		if n < limit {
			limit = n
		}
	}

	logs, total, truncated, err := h.opsService.ListSystemLogsForExport(c.Request.Context(), filter, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Write UTF-8 BOM (EF BB BF) so Excel renders Chinese / multibyte content correctly.
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{
		"created_at",
		"level",
		"component",
		"message",
		"request_id",
		"client_request_id",
		"user_id",
		"account_id",
		"platform",
		"model",
		"extra",
	}); err != nil {
		response.InternalError(c, "Failed to write CSV header: "+err.Error())
		return
	}
	for _, row := range logs {
		userID := ""
		if row.UserID != nil {
			userID = strconv.FormatInt(*row.UserID, 10)
		}
		accountID := ""
		if row.AccountID != nil {
			accountID = strconv.FormatInt(*row.AccountID, 10)
		}
		extraJSON := ""
		if len(row.Extra) > 0 {
			if raw, mErr := json.Marshal(row.Extra); mErr == nil {
				extraJSON = string(raw)
			}
		}
		if err := w.Write([]string{
			row.CreatedAt.UTC().Format(time.RFC3339),
			row.Level,
			row.Component,
			row.Message,
			row.RequestID,
			row.ClientRequestID,
			userID,
			accountID,
			row.Platform,
			row.Model,
			extraJSON,
		}); err != nil {
			response.InternalError(c, "Failed to write CSV row: "+err.Error())
			return
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		response.InternalError(c, "Failed to flush CSV: "+err.Error())
		return
	}

	filename := fmt.Sprintf("ops-system-logs-%s.csv", time.Now().UTC().Format("20060102-150405"))
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("X-Export-Total", strconv.Itoa(total))
	if truncated {
		c.Header("X-Export-Truncated", "true")
	}
	c.Header("Access-Control-Expose-Headers", "Content-Disposition, X-Export-Total, X-Export-Truncated")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
