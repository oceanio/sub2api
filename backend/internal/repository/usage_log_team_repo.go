package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NewTeamUsageRepository exposes the team-scoped slice of usageLogRepository
// as service.TeamUsageRepository. Uses the same concrete struct as
// NewUsageLogRepository so the wire layer can hand the same instance to both
// interfaces.
func NewTeamUsageRepository(client *dbent.Client, sqlDB *sql.DB) service.TeamUsageRepository {
	return newUsageLogRepositoryWithSQL(client, sqlDB)
}

// usage_log_team_repo.go intentionally duplicates the team-scoped query
// variants of the dashboard/stats methods that already exist in
// usage_log_repo.go. The duplication is the price of isolation: the team
// feature can be added to a fork without modifying a single line of the
// admin/global usage_log_repo.go.
//
// Every method here applies the `user_id IN (SELECT user_id FROM team_members
// WHERE team_id = $N AND deleted_at IS NULL)` subquery so the SQL text stays
// fixed-size regardless of team size and the planner can use the
// team_members(team_id) index.

// ── helpers ────────────────────────────────────────────────────────────────

// appendTeamMemberWhereCondition adds the team-scope subquery to a conditions
// list (the shape used by ListWithFilters-style query builders). Returns
// (conditions, args) unchanged when teamID is nil/<=0.
func appendTeamMemberWhereCondition(conditions []string, args []any, teamID *int64) ([]string, []any) {
	if teamID == nil || *teamID <= 0 {
		return conditions, args
	}
	conditions = append(conditions,
		fmt.Sprintf("user_id IN (SELECT user_id FROM team_members WHERE team_id = $%d AND deleted_at IS NULL)", len(args)+1))
	args = append(args, *teamID)
	return conditions, args
}

// appendTeamMemberQueryFilter adds the team-scope subquery to an inline query
// string (the shape used by the dashboard stats methods). userColExpr is
// "user_id" by default or "ul.user_id" when the table is aliased as ul.
func appendTeamMemberQueryFilter(query string, args []any, teamID int64, userColExpr string) (string, []any) {
	if teamID <= 0 {
		return query, args
	}
	if userColExpr == "" {
		userColExpr = "user_id"
	}
	query += fmt.Sprintf(" AND %s IN (SELECT user_id FROM team_members WHERE team_id = $%d AND deleted_at IS NULL)", userColExpr, len(args)+1)
	args = append(args, teamID)
	return query, args
}

// teamID returns the dereferenced TeamID from a UsageLogFilters, or 0 if nil.
func teamIDFromFilters(filters usagestats.UsageLogFilters) int64 {
	if filters.TeamID == nil {
		return 0
	}
	return *filters.TeamID
}

// ── List / Stats (team-scoped equivalents of ListWithFilters / GetStatsWithFilters) ──

// ListTeamUsageLogs lists usage logs scoped to a team. Honors all UsageLogFilters
// fields except AccountID (intentionally ignored — team usage must not depend
// on upstream-account data). Adds an api_key name substring filter when
// filters.APIKeyName is set, since the team UI offers free-text key search.
func (r *usageLogRepository) ListTeamUsageLogs(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
	}
	conditions, args = appendTeamMemberWhereCondition(conditions, args, filters.TeamID)
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
	}
	if strings.TrimSpace(filters.APIKeyName) != "" {
		conditions = append(conditions,
			fmt.Sprintf("api_key_id IN (SELECT id FROM api_keys WHERE name ILIKE $%d AND deleted_at IS NULL)", len(args)+1))
		args = append(args, "%"+strings.TrimSpace(filters.APIKeyName)+"%")
	}
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
	}
	conditions, args = appendRawUsageLogModelWhereCondition(conditions, args, filters.Model)
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
	}
	conditions, args = appendUsageLogBillingModeWhereCondition(conditions, args, filters.BillingMode)
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
	}
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *filters.EndTime)
	}

	whereClause := buildWhere(conditions)
	logs, page, err := r.listUsageLogsWithPagination(ctx, whereClause, args, params)
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateUsageLogAssociations(ctx, logs); err != nil {
		return nil, nil, err
	}
	return logs, page, nil
}

// GetTeamUsageStats returns the team's aggregate totals (no endpoint
// distribution — cheaper than GetStatsWithFilters by 3 queries since the
// team's usage UI shows endpoint stats via a separate endpoint).
func (r *usageLogRepository) GetTeamUsageStats(ctx context.Context, filters usagestats.UsageLogFilters) (*UsageStats, error) {
	conditions := make([]string, 0, 9)
	args := make([]any, 0, 9)

	if filters.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, filters.UserID)
	}
	conditions, args = appendTeamMemberWhereCondition(conditions, args, filters.TeamID)
	if filters.APIKeyID > 0 {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", len(args)+1))
		args = append(args, filters.APIKeyID)
	}
	if strings.TrimSpace(filters.APIKeyName) != "" {
		conditions = append(conditions,
			fmt.Sprintf("api_key_id IN (SELECT id FROM api_keys WHERE name ILIKE $%d AND deleted_at IS NULL)", len(args)+1))
		args = append(args, "%"+strings.TrimSpace(filters.APIKeyName)+"%")
	}
	if filters.GroupID > 0 {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
		args = append(args, filters.GroupID)
	}
	conditions, args = appendRawUsageLogModelWhereCondition(conditions, args, filters.Model)
	conditions, args = appendRequestTypeOrStreamWhereCondition(conditions, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", len(args)+1))
		args = append(args, int16(*filters.BillingType))
	}
	conditions, args = appendUsageLogBillingModeWhereCondition(conditions, args, filters.BillingMode)
	if filters.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *filters.StartTime)
	}
	if filters.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *filters.EndTime)
	}

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0) as total_cache_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			COALESCE(SUM(actual_cost), 0) as total_actual_cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as total_account_cost,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM usage_logs
		%s
	`, buildWhere(conditions))

	stats := &UsageStats{}
	var totalAccountCost float64
	if err := scanSingleRow(
		ctx, r.sql, query, args,
		&stats.TotalRequests,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalCacheTokens,
		&stats.TotalCost,
		&stats.TotalActualCost,
		&totalAccountCost,
		&stats.AverageDurationMs,
	); err != nil {
		return nil, err
	}
	stats.TotalAccountCost = &totalAccountCost
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheTokens
	return stats, nil
}

// ── Chart aggregations ─────────────────────────────────────────────────────

// GetTeamUsageTrend returns the team's per-bucket usage trend.
func (r *usageLogRepository) GetTeamUsageTrend(ctx context.Context, filters usagestats.UsageLogFilters, granularity string) (results []TrendDataPoint, err error) {
	dateFormat := safeDateFormat(granularity)
	startTime, endTime := timeRange(filters)
	teamID := teamIDFromFilters(filters)

	query := fmt.Sprintf(`
		SELECT
			TO_CHAR(created_at, '%s') as date,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, dateFormat)

	args := []any{startTime, endTime}
	query, args = applyTeamCommonInlineFilters(query, args, filters, teamID, "user_id")
	query += " GROUP BY date ORDER BY date ASC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	return scanTrendRows(rows)
}

// GetTeamModelStats returns the team's per-model usage breakdown for a given
// model dimension (requested/upstream/mapping).
func (r *usageLogRepository) GetTeamModelStats(ctx context.Context, filters usagestats.UsageLogFilters, source string) (results []ModelStat, err error) {
	modelExpr := resolveModelDimensionExpression(source)
	startTime, endTime := timeRange(filters)
	teamID := teamIDFromFilters(filters)

	query := fmt.Sprintf(`
		SELECT
			%s as model,
			COUNT(*) as requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) as account_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, modelExpr)

	args := []any{startTime, endTime}
	query, args = applyTeamCommonInlineFilters(query, args, filters, teamID, "user_id")
	query += fmt.Sprintf(" GROUP BY %s ORDER BY total_tokens DESC", modelExpr)

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	return scanModelStatsRows(rows)
}

// GetTeamGroupStats returns the team's per-group usage breakdown.
func (r *usageLogRepository) GetTeamGroupStats(ctx context.Context, filters usagestats.UsageLogFilters) (results []usagestats.GroupStat, err error) {
	startTime, endTime := timeRange(filters)
	teamID := teamIDFromFilters(filters)

	query := `
		SELECT
			COALESCE(ul.group_id, 0) as group_id,
			COALESCE(g.name, '') as group_name,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost,
			COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) as account_cost
		FROM usage_logs ul
		LEFT JOIN groups g ON g.id = ul.group_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`

	args := []any{startTime, endTime}
	query, args = applyTeamCommonInlineFilters(query, args, filters, teamID, "ul.user_id")
	query += " GROUP BY ul.group_id, g.name ORDER BY total_tokens DESC"

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]usagestats.GroupStat, 0)
	for rows.Next() {
		var row usagestats.GroupStat
		if err := rows.Scan(&row.GroupID, &row.GroupName, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost, &row.AccountCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// GetTeamEndpointStats returns inbound endpoint and inbound->upstream-path
// breakdowns for the team. The upstream-only column is intentionally omitted
// from the team view to avoid exposing upstream account identifiers.
func (r *usageLogRepository) GetTeamEndpointStats(ctx context.Context, filters usagestats.UsageLogFilters) (inbound []EndpointStat, paths []EndpointStat, err error) {
	startTime, endTime := timeRange(filters)
	teamID := teamIDFromFilters(filters)

	if inbound, err = r.teamEndpointStatsByColumn(ctx, "inbound_endpoint", startTime, endTime, teamID, filters); err != nil {
		return nil, nil, fmt.Errorf("inbound endpoint stats: %w", err)
	}
	if paths, err = r.teamEndpointPathStats(ctx, startTime, endTime, teamID, filters); err != nil {
		return nil, nil, fmt.Errorf("endpoint path stats: %w", err)
	}
	return inbound, paths, nil
}

func (r *usageLogRepository) teamEndpointStatsByColumn(ctx context.Context, endpointColumn string, startTime, endTime time.Time, teamID int64, filters usagestats.UsageLogFilters) (results []EndpointStat, err error) {
	query := fmt.Sprintf(`
		SELECT
			COALESCE(NULLIF(TRIM(%s), ''), 'unknown') AS endpoint,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`, endpointColumn)

	args := []any{startTime, endTime}
	query, args = applyTeamCommonInlineFilters(query, args, filters, teamID, "user_id")
	query += " GROUP BY endpoint ORDER BY requests DESC"

	return scanEndpointRows(ctx, r.sql, query, args)
}

func (r *usageLogRepository) teamEndpointPathStats(ctx context.Context, startTime, endTime time.Time, teamID int64, filters usagestats.UsageLogFilters) (results []EndpointStat, err error) {
	query := `
		SELECT
			CONCAT(
				COALESCE(NULLIF(TRIM(inbound_endpoint), ''), 'unknown'),
				' -> ',
				COALESCE(NULLIF(TRIM(upstream_endpoint), ''), 'unknown')
			) AS endpoint,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) AS total_tokens,
			COALESCE(SUM(total_cost), 0) as cost,
			COALESCE(SUM(actual_cost), 0) as actual_cost
		FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2
	`
	args := []any{startTime, endTime}
	query, args = applyTeamCommonInlineFilters(query, args, filters, teamID, "user_id")
	query += " GROUP BY endpoint ORDER BY requests DESC"

	return scanEndpointRows(ctx, r.sql, query, args)
}

// GetTeamUserBreakdown returns per-user breakdown within a dimension, scoped
// to the team. dim.TeamID is honored as the team scope.
func (r *usageLogRepository) GetTeamUserBreakdown(ctx context.Context, filters usagestats.UsageLogFilters, dim usagestats.UserBreakdownDimension, limit int) (results []usagestats.UserBreakdownItem, err error) {
	startTime, endTime := timeRange(filters)
	teamID := teamIDFromFilters(filters)
	if dim.TeamID == 0 {
		dim.TeamID = teamID
	}

	query := `
		SELECT
			COALESCE(ul.user_id, 0) as user_id,
			COALESCE(u.email, '') as email,
			COUNT(*) as requests,
			COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(ul.total_cost), 0) as cost,
			COALESCE(SUM(ul.actual_cost), 0) as actual_cost,
			COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) as account_cost
		FROM usage_logs ul
		LEFT JOIN users u ON u.id = ul.user_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`
	args := []any{startTime, endTime}

	query, args = appendTeamMemberQueryFilter(query, args, dim.TeamID, "ul.user_id")
	if dim.GroupID > 0 {
		query += fmt.Sprintf(" AND ul.group_id = $%d", len(args)+1)
		args = append(args, dim.GroupID)
	}
	if dim.Model != "" {
		query += fmt.Sprintf(" AND %s = $%d", resolveModelDimensionExpression(dim.ModelType), len(args)+1)
		args = append(args, dim.Model)
	}
	if dim.Endpoint != "" {
		query += fmt.Sprintf(" AND %s = $%d", resolveEndpointColumn(dim.EndpointType), len(args)+1)
		args = append(args, dim.Endpoint)
	}
	if dim.UserID > 0 {
		query += fmt.Sprintf(" AND ul.user_id = $%d", len(args)+1)
		args = append(args, dim.UserID)
	}
	if dim.APIKeyID > 0 {
		query += fmt.Sprintf(" AND ul.api_key_id = $%d", len(args)+1)
		args = append(args, dim.APIKeyID)
	}
	if dim.RequestType != nil {
		query += fmt.Sprintf(" AND ul.request_type = $%d", len(args)+1)
		args = append(args, *dim.RequestType)
	}
	if dim.Stream != nil {
		query += fmt.Sprintf(" AND ul.stream = $%d", len(args)+1)
		args = append(args, *dim.Stream)
	}
	if dim.BillingType != nil {
		query += fmt.Sprintf(" AND ul.billing_type = $%d", len(args)+1)
		args = append(args, *dim.BillingType)
	}

	query += " GROUP BY ul.user_id, u.email ORDER BY actual_cost DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]usagestats.UserBreakdownItem, 0)
	for rows.Next() {
		var row usagestats.UserBreakdownItem
		if err := rows.Scan(&row.UserID, &row.Email, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost, &row.AccountCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// ── shared helpers ─────────────────────────────────────────────────────────

// applyTeamCommonInlineFilters appends the common 6 inline filter conditions
// (user_id / api_key_id / api_key_name / group_id / model / request_type /
// billing_type / billing_mode) plus the team-scope subquery. AccountID is
// intentionally not honored. Used by the chart aggregation methods that build
// queries inline (rather than via a conditions slice).
func applyTeamCommonInlineFilters(query string, args []any, filters usagestats.UsageLogFilters, teamID int64, userColExpr string) (string, []any) {
	if filters.UserID > 0 {
		query += fmt.Sprintf(" AND %s = $%d", userColExpr, len(args)+1)
		args = append(args, filters.UserID)
	}
	query, args = appendTeamMemberQueryFilter(query, args, teamID, userColExpr)
	if filters.APIKeyID > 0 {
		query += fmt.Sprintf(" AND api_key_id = $%d", len(args)+1)
		args = append(args, filters.APIKeyID)
	}
	if strings.TrimSpace(filters.APIKeyName) != "" {
		query += fmt.Sprintf(" AND api_key_id IN (SELECT id FROM api_keys WHERE name ILIKE $%d AND deleted_at IS NULL)", len(args)+1)
		args = append(args, "%"+strings.TrimSpace(filters.APIKeyName)+"%")
	}
	if filters.GroupID > 0 {
		query += fmt.Sprintf(" AND group_id = $%d", len(args)+1)
		args = append(args, filters.GroupID)
	}
	query, args = appendRawUsageLogModelQueryFilter(query, args, filters.Model)
	query, args = appendRequestTypeOrStreamQueryFilter(query, args, filters.RequestType, filters.Stream)
	if filters.BillingType != nil {
		query += fmt.Sprintf(" AND billing_type = $%d", len(args)+1)
		args = append(args, int16(*filters.BillingType))
	}
	if filters.BillingMode != "" {
		query += fmt.Sprintf(" AND billing_mode = $%d", len(args)+1)
		args = append(args, filters.BillingMode)
	}
	return query, args
}

// timeRange returns the half-open time bounds derived from the filter set,
// substituting epoch / now when either side is missing.
func timeRange(filters usagestats.UsageLogFilters) (time.Time, time.Time) {
	start := time.Unix(0, 0).UTC()
	if filters.StartTime != nil {
		start = *filters.StartTime
	}
	end := time.Now().UTC()
	if filters.EndTime != nil {
		end = *filters.EndTime
	}
	return start, end
}

func scanEndpointRows(ctx context.Context, q sqlExecutor, query string, args []any) (results []EndpointStat, err error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()
	results = make([]EndpointStat, 0)
	for rows.Next() {
		var row EndpointStat
		if err := rows.Scan(&row.Endpoint, &row.Requests, &row.TotalTokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
