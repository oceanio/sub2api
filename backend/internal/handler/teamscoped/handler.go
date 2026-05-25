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
	"context"
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

// operatorID returns the authenticated user_id of whoever made the request.
// Always the real user — sys admin or team_admin alike — so audit log
// columns with FK to users (e.g. team_balance_logs.operator_id) stay valid.
// The bypass for sys admin's team_admin membership check is signalled via
// the request context instead (see reqCtx / service.WithSysAdminCaller).
func operatorID(c *gin.Context) int64 {
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	return subject.UserID
}

// reqCtx returns the request context, marked as a sys-admin caller for
// /api/v1/admin/* routes so service.requireTeamAdmin skips the team_admins
// membership check (the admin-auth middleware has already verified sys admin
// access on these routes).
func reqCtx(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if strings.HasPrefix(c.FullPath(), "/api/v1/admin/") {
		ctx = service.WithSysAdminCaller(ctx)
	}
	return ctx
}

// ── Team detail / metadata ───────────────────────────────────────────────────

func (h *Handler) GetTeam(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	team, err := h.teamService.GetTeam(reqCtx(c), teamID)
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
	team, err := h.teamService.UpdateAvailableTags(reqCtx(c), teamID, operatorID(c), cleaned)
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
	filters := service.TeamMemberListFilters{
		Tags:   cleanTags,
		Search: c.Query("search"),
	}

	members, result, err := h.teamService.ListMembersFiltered(reqCtx(c), teamID, operatorID(c), filters, params)
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
	member, err := h.teamService.UpdateMemberSubQuota(reqCtx(c), service.UpdateTeamMemberSubQuotaRequest{
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
	if err := h.teamService.RemoveMember(reqCtx(c), service.RemoveTeamMemberRequest{
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
	admins, err := h.teamService.ListTeamAdmins(reqCtx(c), teamID)
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
	ta, err := h.teamService.AddTeamAdmin(reqCtx(c), service.AddTeamAdminRequest{
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
	if err := h.teamService.RemoveTeamAdmin(reqCtx(c), service.RemoveTeamAdminRequest{
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
	records, result, err := h.teamService.ListUsageLogs(reqCtx(c), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.UsageLog, 0, len(records))
	for i := range records {
		out = append(out, *dto.UsageLogFromService(&records[i]))
	}
	stripUsageLogsCost(out)
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
	stats, err := h.teamService.GetUsageStats(reqCtx(c), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	stripUsageStatsCost(stats)
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
		trend, err := h.teamService.GetTeamUsageTrend(reqCtx(c), filters, granularity)
		if err != nil {
			return nil, err
		}
		stripTrendCost(trend)
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
		stats, err := h.teamService.GetTeamModelStats(reqCtx(c), filters, modelSource)
		if err != nil {
			return nil, err
		}
		stripModelStatsCost(stats)
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
		stats, err := h.teamService.GetTeamGroupStats(reqCtx(c), filters)
		if err != nil {
			return nil, err
		}
		stripGroupStatsCost(stats)
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
		inbound, paths, err := h.teamService.GetTeamEndpointStats(reqCtx(c), filters)
		if err != nil {
			return nil, err
		}
		stripEndpointStatsCost(inbound)
		stripEndpointStatsCost(paths)
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
	stats, err := h.teamService.GetTeamUserBreakdown(reqCtx(c), filters, dim, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	stripUserBreakdownCost(stats)
	response.Success(c, gin.H{"users": stats})
}

// ── Member individual ops (mirrored from old TeamHandler so sys admin and
//    team_admin share the same impl + permission path) ─────────────────────

type createMemberRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Username string `json:"username"`
}

type batchCreateMembersRequest struct {
	Rows []createMemberRequest `json:"rows" binding:"required,min=1,max=200,dive"`
}

// BulkCreateMembers POST .../members/batch — create N new users and add them
// to the team. Each row is its own tx; the response carries per-row outcome
// so the frontend can show "47 succeeded, 3 failed" and let the operator fix
// just the failed rows.
func (h *Handler) BulkCreateMembers(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	var req batchCreateMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	rows := make([]service.BulkCreateMemberRow, 0, len(req.Rows))
	for _, r := range req.Rows {
		rows = append(rows, service.BulkCreateMemberRow{
			Email:    r.Email,
			Password: r.Password,
			Username: r.Username,
		})
	}
	result, err := h.teamService.BulkCreateMembersUsers(reqCtx(c), teamID, rows, operatorID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// CreateMember POST /api/v1/team/:teamId/members
//          and POST /api/v1/admin/teams/:id/members/new
// Creates a NEW user account and adds them to the team in one transaction.
// (Sys admin's old AddMember endpoint adds an EXISTING user by id and stays
//  on /admin/teams/:id/members.)
func (h *Handler) CreateMember(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	var req createMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	member, err := h.teamService.CreateMemberUser(reqCtx(c), service.CreateTeamMemberUserRequest{
		TeamID:     teamID,
		Email:      req.Email,
		Password:   req.Password,
		Username:   req.Username,
		OperatorID: operatorID(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

// GetMember GET /api/v1/{team|admin/teams}/:id/members/:memberID
func (h *Handler) GetMember(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	member, err := h.teamService.GetMember(reqCtx(c), teamID, memberID, operatorID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

type updateMemberTagsRequest struct {
	Tags []string `json:"tags"`
}

// UpdateMemberTags PUT .../members/:memberID/tags
func (h *Handler) UpdateMemberTags(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	var req updateMemberTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	member, err := h.teamService.UpdateMemberTags(reqCtx(c), teamID, memberID, operatorID(c), req.Tags)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

type setMemberStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// SetMemberStatus PUT .../members/:memberID/status
func (h *Handler) SetMemberStatus(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	var req setMemberStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.teamService.SetMemberStatus(reqCtx(c), teamID, memberID, operatorID(c), req.Status); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "status updated"})
}

type resetMemberPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ResetMemberPassword PUT .../members/:memberID/password
func (h *Handler) ResetMemberPassword(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	var req resetMemberPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.teamService.ResetMemberPassword(reqCtx(c), teamID, memberID, operatorID(c), req.NewPassword); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "password reset"})
}

// ListMemberAPIKeys GET .../members/:memberID/api-keys
func (h *Handler) ListMemberAPIKeys(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	keys, err := h.teamService.ListMemberAPIKeys(reqCtx(c), teamID, memberID, operatorID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Success(c, out)
}

// ListMemberSubscriptions GET .../members/:memberID/subscriptions
func (h *Handler) ListMemberSubscriptions(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	subs, err := h.teamService.ListMemberSubscriptions(reqCtx(c), teamID, memberID, operatorID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// Same DTO wrap as ListSubscriptions — service.UserSubscription has no JSON tags.
	out := make([]dto.AdminUserSubscription, 0, len(subs))
	for i := range subs {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subs[i]))
	}
	response.Success(c, out)
}

// ── Subscriptions / balance ──────────────────────────────────────────────────

func (h *Handler) ListSubscriptions(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	filters := service.UserSubscriptionTeamFilters{
		Status: c.Query("status"),
		Search: c.Query("search"),
	}
	if g := c.Query("group_id"); g != "" {
		if id, err := strconv.ParseInt(g, 10, 64); err == nil {
			filters.GroupID = &id
		}
	}
	subs, result, err := h.teamService.ListTeamSubscriptions(reqCtx(c), teamID, operatorID(c), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// service.UserSubscription has no JSON tags, so wrap in the admin DTO that
	// has the snake_case keys the frontend expects.
	out := make([]dto.AdminUserSubscription, 0, len(subs))
	for i := range subs {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subs[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

type resetSubscriptionQuotaRequest struct {
	Daily   bool `json:"daily"`
	Weekly  bool `json:"weekly"`
	Monthly bool `json:"monthly"`
}

// ResetSubscriptionQuota POST .../subscriptions/:subID/reset-quota
func (h *Handler) ResetSubscriptionQuota(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	subID, err := strconv.ParseInt(c.Param("subID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid subscription id")
		return
	}
	var req resetSubscriptionQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	sub, err := h.teamService.ResetTeamSubscriptionQuota(reqCtx(c), teamID, subID, operatorID(c), req.Daily, req.Weekly, req.Monthly)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.UserSubscriptionFromServiceAdmin(sub))
}

// RevokeSubscription DELETE .../subscriptions/:subID
func (h *Handler) RevokeSubscription(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	subID, err := strconv.ParseInt(c.Param("subID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid subscription id")
		return
	}
	if err := h.teamService.RevokeTeamSubscription(reqCtx(c), teamID, subID, operatorID(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "subscription revoked"})
}

// ListPlans GET /api/v1/team/:teamId/plans
//       and GET /api/v1/admin/teams/:id/plans
func (h *Handler) ListPlans(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	plans, err := h.teamService.ListSubscriptionPlans(reqCtx(c), teamID, operatorID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]gin.H, 0, len(plans))
	for _, p := range plans {
		out = append(out, gin.H{
			"id":            p.ID,
			"group_id":      p.GroupID,
			"name":          p.Name,
			"description":   p.Description,
			"price":         p.Price,
			"validity_days": p.ValidityDays,
			"product_name":  p.ProductName,
			"for_sale":      p.ForSale,
		})
	}
	response.Success(c, out)
}

type purchaseSubscriptionRequest struct {
	// Exactly one of UserIDs / AllMembers must be set.
	UserIDs    []int64 `json:"user_ids,omitempty" binding:"omitempty,dive,gt=0"`
	AllMembers bool    `json:"all_members,omitempty"`
	PlanID     int64   `json:"plan_id" binding:"required"`
}

// PurchaseSubscription POST /api/v1/team/:teamId/subscriptions
//                  and POST /api/v1/admin/teams/:id/subscriptions
//
// Purchase-or-renew semantics: if a target member already has a subscription
// for the plan's group, the term is extended; otherwise a new subscription is
// created. Work is committed in chunks; a partial run is reported via the
// response's `stopped_reason`.
func (h *Handler) PurchaseSubscription(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	var req purchaseSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.AllMembers && len(req.UserIDs) > 0 {
		response.BadRequest(c, "cannot specify both all_members and user_ids")
		return
	}
	if !req.AllMembers && len(req.UserIDs) == 0 {
		response.BadRequest(c, "user_ids required when all_members is false")
		return
	}
	result, err := h.teamService.PurchaseSubscriptionsForMembers(reqCtx(c), teamID, req.UserIDs, req.AllMembers, req.PlanID, operatorID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ListBalanceLogs(c *gin.Context) {
	teamID, ok := resolveTeamID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	logs, result, err := h.teamService.ListBalanceLogs(reqCtx(c), teamID, operatorID(c), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, logs, result.Total, page, pageSize)
}
