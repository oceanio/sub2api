// Package usagefilter parses the team-scoped usage query string used by both
// /admin/teams/:id/usage/* (sys admin) and /team/:teamId/usage/* (team_admin).
// Intentionally self-contained — does NOT reuse the admin global parser
// (handler/admin/usage_handler.go) so adding the team feature does not couple
// the two parsers across an upstream-volatile file.
package usagefilter

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ParseTeamUsage parses the usage query string for a team-scoped endpoint
// into a UsageLogFilters. Returns (filters, ok). When ok=false the caller
// should stop — the helper has already written either a 400 or, when a
// non-member user_id was passed, an empty-payload response via emptyWriter.
//
// Accepted query params (mirrors the admin /admin/usage/* filter set but
// without account_id, since team usage must not depend on upstream-account
// data):
//
//	user_id, api_key_id, api_key_name, model, group_id, request_type,
//	billing_type, billing_mode, start_date, end_date.
//
// Date range is parsed as UTC (no tz handling — team UI runs on UTC).
// user_id is membership-checked: a non-member id short-circuits with the
// emptyWriter callback (handlers pass different shapes for list vs summary).
// When no user_id filter is given, filters.TeamID is set to scope the query
// via team-members subquery.
func ParseTeamUsage(c *gin.Context, teamService *service.TeamService, teamID int64, emptyWriter func()) (usagestats.UsageLogFilters, bool) {
	filters := usagestats.UsageLogFilters{
		Model:       c.Query("model"),
		APIKeyName:  c.Query("api_key_name"),
		BillingMode: c.Query("billing_mode"),
	}

	if v := c.Query("api_key_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			filters.APIKeyID = id
		}
	}
	if v := c.Query("group_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			filters.GroupID = id
		}
	}
	if v := strings.TrimSpace(c.Query("request_type")); v != "" {
		parsed, err := service.ParseUsageRequestType(v)
		if err != nil {
			response.BadRequest(c, "invalid request_type")
			return filters, false
		}
		rt := int16(parsed)
		filters.RequestType = &rt
	} else if v := strings.TrimSpace(c.Query("stream")); v != "" {
		streamVal, err := strconv.ParseBool(v)
		if err != nil {
			response.BadRequest(c, "invalid stream")
			return filters, false
		}
		filters.Stream = &streamVal
	}
	if v := strings.TrimSpace(c.Query("billing_type")); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 8)
		if err != nil {
			response.BadRequest(c, "invalid billing_type")
			return filters, false
		}
		bt := int8(parsed)
		filters.BillingType = &bt
	}

	if s := c.Query("start_date"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			filters.StartTime = &t
		}
	}
	if s := c.Query("end_date"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			t = t.AddDate(0, 0, 1)
			filters.EndTime = &t
		}
	}

	var userID int64
	if v := c.Query("user_id"); v != "" {
		userID, _ = strconv.ParseInt(v, 10, 64)
	}
	if userID > 0 {
		ok, err := teamService.IsTeamMember(c.Request.Context(), teamID, userID)
		if err != nil {
			response.ErrorFrom(c, err)
			return filters, false
		}
		if !ok {
			if emptyWriter != nil {
				emptyWriter()
			}
			return filters, false
		}
		filters.UserID = userID
	} else {
		filters.TeamID = &teamID
	}
	return filters, true
}
