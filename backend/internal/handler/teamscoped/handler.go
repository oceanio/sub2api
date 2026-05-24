// Package teamscoped holds the handler methods that are shared between the
// sys-admin (`/admin/teams/:id/*`) and team_admin (`/team/:teamId/*`) URL
// families. The two URL trees expose the same data shapes; the only thing that
// differs is who is allowed to call (route middleware) and the source of the
// team id (URL parameter name).
//
// Handler implementations resolve those two differences here:
//
//   - teamID is read from `:teamId` (team_admin) or `:id` (sys admin).
//   - operatorID is the auth subject's user id for team_admin requests, and
//     0 for sys admin requests. Service methods that gate on team_admin
//     membership treat 0 as a trusted-caller bypass — the route middleware
//     has already verified sys admin authority.
//
// Lives in its own subpackage so both the parent `handler` package and the
// `handler/admin` package can import it without an import cycle.
package teamscoped

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/handler/usagefilter"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 30s read-through caches for the team-scoped chart endpoints. Same shape as
// the admin dashboard cache (handler/admin/dashboard_query_cache.go) — every
// chart payload is keyed by (teamID, filter fingerprint) so a filter flick on
// the UI only pays for the first miss within the TTL window. Implementation
// is duplicated locally (snapshot_cache.go in this package) so adding the
// team feature does not touch any admin file.
var (
	teamTrendCache         = newSnapshotCache(30 * time.Second)
	teamModelStatsCache    = newSnapshotCache(30 * time.Second)
	teamGroupStatsCache    = newSnapshotCache(30 * time.Second)
	teamEndpointStatsCache = newSnapshotCache(30 * time.Second)
)

// buildTeamChartCacheKey serialises the filter set into a deterministic key.
// The teamID is implicit in filters.TeamID/filters.UserID — when neither is
// set the helper is being called outside a team scope, which should not
// happen (the handler short-circuits earlier) and yields an empty key.
func buildTeamChartCacheKey(label string, filters usagestats.UsageLogFilters, extras ...string) string {
	type chartCacheKey struct {
		Label       string
		StartTime   string
		EndTime     string
		TeamID      int64
		UserID      int64
		APIKeyID    int64
		APIKeyName  string
		GroupID     int64
		Model       string
		BillingMode string
		RequestType *int16
		Stream      *bool
		BillingType *int8
		Extras      []string
	}
	key := chartCacheKey{
		Label:       label,
		UserID:      filters.UserID,
		APIKeyID:    filters.APIKeyID,
		APIKeyName:  filters.APIKeyName,
		GroupID:     filters.GroupID,
		Model:       filters.Model,
		BillingMode: filters.BillingMode,
		RequestType: filters.RequestType,
		Stream:      filters.Stream,
		BillingType: filters.BillingType,
		Extras:      extras,
	}
	if filters.StartTime != nil {
		key.StartTime = filters.StartTime.UTC().Format(time.RFC3339)
	}
	if filters.EndTime != nil {
		key.EndTime = filters.EndTime.UTC().Format(time.RFC3339)
	}
	if filters.TeamID != nil {
		key.TeamID = *filters.TeamID
	}
	raw, _ := json.Marshal(key)
	return string(raw)
}

// Handler hosts every team-scoped endpoint shared by /admin/teams/:id/* and
// /team/:teamId/*.
type Handler struct {
	teamService *service.TeamService
}

func NewHandler(teamService *service.TeamService) *Handler {
	return &Handler{teamService: teamService}
}

// resolveTeamID reads the team id from whichever URL param the route uses.
// Returns (id, true) on success; on failure it has already written a 400.
func resolveTeamID(c *gin.Context) (int64, bool) {
	for _, name := range []string{"teamId", "id"} {
		if v := c.Param(name); v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil && id > 0 {
				return id, true
			}
		}
	}
	response.BadRequest(c, "invalid team id")
	return 0, false
}

// operatorID returns the team_admin operator id for service-layer permission
// checks. For requests routed through /admin/teams/* the route middleware has
// already confirmed sys admin, so we return 0 to signal the bypass. For
// /team/* requests we forward the authenticated user id so the service layer
// re-verifies the team_admin row.
func operatorID(c *gin.Context) int64 {
	if strings.HasPrefix(c.FullPath(), "/api/v1/admin/") {
		return 0
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	return subject.UserID
}

// ── Team detail / metadata ───────────────────────────────────────────────────

func (h *Handler) GetTeam(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	team, err := h.teamService.GetTeam(c.Request.Context(), teamID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

type updateAvailableTagsRequest struct {
	Tags []string `json:"tags"`
}

// trimAndSimplifyTag trims surrounding whitespace and caps length by runes.
// (Bytes would risk splitting a multi-byte rune at the boundary.)
func trimAndSimplifyTag(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 50 {
		s = string(runes[:50])
	}
	return s
}

func (h *Handler) UpdateAvailableTags(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	var req updateAvailableTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	seen := make(map[string]struct{})
	cleaned := make([]string, 0, len(req.Tags))
	for _, raw := range req.Tags {
		t := trimAndSimplifyTag(raw)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		cleaned = append(cleaned, t)
	}
	team, err := h.teamService.UpdateAvailableTags(c.Request.Context(), teamID, operatorID(c), cleaned)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

// ── Members ──────────────────────────────────────────────────────────────────

func (h *Handler) ListMembers(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "asc"}

	tags := c.QueryArray("tag")
	cleanTags := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != "" {
			cleanTags = append(cleanTags, t)
		}
	}

	members, result, err := h.teamService.ListMembersFiltered(c.Request.Context(), teamID, operatorID(c), cleanTags, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, members, result.Total, page, pageSize)
}

type updateSubQuotaRequest struct {
	SubQuota float64 `json:"sub_quota" binding:"gte=0"`
}

func (h *Handler) UpdateMemberSubQuota(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	var req updateSubQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	member, err := h.teamService.UpdateMemberSubQuota(c.Request.Context(), service.UpdateTeamMemberSubQuotaRequest{
		TeamID:     teamID,
		MemberID:   memberID,
		SubQuota:   req.SubQuota,
		OperatorID: operatorID(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

func (h *Handler) RemoveMember(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	if err := h.teamService.RemoveMember(c.Request.Context(), service.RemoveTeamMemberRequest{
		TeamID:     teamID,
		MemberID:   memberID,
		OperatorID: operatorID(c),
	}); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "member removed"})
}

// ── Admins (the team_admins role table — orthogonal to sys admin) ────────────

func (h *Handler) ListAdmins(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	admins, err := h.teamService.ListTeamAdmins(c.Request.Context(), teamID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, admins)
}

type adminUserIDRequest struct {
	UserID int64 `json:"user_id" binding:"required"`
}

func (h *Handler) AddAdmin(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	var req adminUserIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	ta, err := h.teamService.AddTeamAdmin(c.Request.Context(), service.AddTeamAdminRequest{
		TeamID:     teamID,
		UserID:     req.UserID,
		OperatorID: subject.UserID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, ta)
}

func (h *Handler) RemoveAdmin(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	if err := h.teamService.RemoveTeamAdmin(c.Request.Context(), service.RemoveTeamAdminRequest{
		TeamID:     teamID,
		UserID:     userID,
		OperatorID: subject.UserID,
	}); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "admin removed"})
}

// ── Usage ────────────────────────────────────────────────────────────────────

func (h *Handler) ListUsage(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Paginated(c, []any{}, 0, page, pageSize)
	})
	if !ok {
		return
	}

	params := pagination.PaginationParams{
		Page: page, PageSize: pageSize,
		SortBy: "created_at", SortOrder: "desc",
	}
	records, result, err := h.teamService.ListUsageLogs(c.Request.Context(), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.UsageLog, 0, len(records))
	for i := range records {
		out = append(out, *dto.UsageLogFromService(&records[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

func (h *Handler) UsageStats(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Success(c, gin.H{"total_requests": 0})
	})
	if !ok {
		return
	}
	stats, err := h.teamService.GetUsageStats(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

func (h *Handler) UsageTrend(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	granularity := strings.TrimSpace(c.DefaultQuery("granularity", "day"))
	if granularity != "hour" {
		granularity = "day"
	}
	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Success(c, gin.H{"trend": []any{}})
	})
	if !ok {
		return
	}
	key := buildTeamChartCacheKey("trend", filters, granularity)
	entry, _, err := teamTrendCache.GetOrLoad(key, func() (any, error) {
		trend, err := h.teamService.GetTeamUsageTrend(c.Request.Context(), filters, granularity)
		if err != nil {
			return nil, err
		}
		return gin.H{"trend": trend}, nil
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entry.Payload)
}

func (h *Handler) UsageModelStats(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	modelSource := c.DefaultQuery("model_source", "requested")
	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Success(c, gin.H{"models": []any{}})
	})
	if !ok {
		return
	}
	key := buildTeamChartCacheKey("model-stats", filters, modelSource)
	entry, _, err := teamModelStatsCache.GetOrLoad(key, func() (any, error) {
		stats, err := h.teamService.GetTeamModelStats(c.Request.Context(), filters, modelSource)
		if err != nil {
			return nil, err
		}
		return gin.H{"models": stats}, nil
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entry.Payload)
}

func (h *Handler) UsageGroupStats(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Success(c, gin.H{"groups": []any{}})
	})
	if !ok {
		return
	}
	key := buildTeamChartCacheKey("group-stats", filters)
	entry, _, err := teamGroupStatsCache.GetOrLoad(key, func() (any, error) {
		stats, err := h.teamService.GetTeamGroupStats(c.Request.Context(), filters)
		if err != nil {
			return nil, err
		}
		return gin.H{"groups": stats}, nil
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entry.Payload)
}

func (h *Handler) UsageEndpointStats(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Success(c, gin.H{"endpoints": []any{}, "endpoint_paths": []any{}})
	})
	if !ok {
		return
	}
	key := buildTeamChartCacheKey("endpoint-stats", filters)
	entry, _, err := teamEndpointStatsCache.GetOrLoad(key, func() (any, error) {
		inbound, paths, err := h.teamService.GetTeamEndpointStats(c.Request.Context(), filters)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"endpoints":      inbound,
			"endpoint_paths": paths,
		}, nil
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, entry.Payload)
}

func (h *Handler) UsageUserBreakdown(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	dim := usagestats.UserBreakdownDimension{}
	if v := c.Query("group_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			dim.GroupID = id
		}
	}
	dim.Model = c.Query("model")
	rawModelSource := strings.TrimSpace(c.DefaultQuery("model_source", usagestats.ModelSourceRequested))
	if !usagestats.IsValidModelSource(rawModelSource) {
		response.BadRequest(c, "Invalid model_source, use requested/upstream/mapping")
		return
	}
	dim.ModelType = rawModelSource
	dim.Endpoint = c.Query("endpoint")
	dim.EndpointType = c.DefaultQuery("endpoint_type", "inbound")

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	filters, ok := usagefilter.ParseTeamUsage(c, h.teamService, teamID, func() {
		response.Success(c, gin.H{"users": []any{}})
	})
	if !ok {
		return
	}
	stats, err := h.teamService.GetTeamUserBreakdown(c.Request.Context(), filters, dim, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"users": stats})
}

// ── Subscriptions / balance ──────────────────────────────────────────────────

func (h *Handler) ListSubscriptions(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	subs, result, err := h.teamService.ListTeamSubscriptions(c.Request.Context(), teamID, operatorID(c), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, subs, result.Total, page, pageSize)
}

func (h *Handler) ListBalanceLogs(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	logs, result, err := h.teamService.ListBalanceLogs(c.Request.Context(), teamID, operatorID(c), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, logs, result.Total, page, pageSize)
}
