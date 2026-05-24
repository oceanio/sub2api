package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// TeamHandler hosts the team_admin-only team endpoints. Shared endpoints
// (also exposed under /admin/teams/*) live in handler/teamscoped.
type TeamHandler struct {
	teamService *service.TeamService
}

func NewTeamHandler(teamService *service.TeamService) *TeamHandler {
	return &TeamHandler{teamService: teamService}
}

// primaryTeamID returns the URL-resolved teamId set by TeamAuthMiddleware.
func (h *TeamHandler) primaryTeamID(c *gin.Context) (int64, bool) {
	teamID, ok := middleware2.GetActiveTeamID(c)
	if !ok {
		response.BadRequest(c, "team id required")
	}
	return teamID, ok
}

func (h *TeamHandler) subject(c *gin.Context) int64 {
	s, _ := middleware2.GetAuthSubjectFromContext(c)
	return s.UserID
}

// ── Cross-team endpoints (no :teamId) ─────────────────────────────────────────

// GetMyTeams GET /api/v1/team/teams — for multi-team admins.
func (h *TeamHandler) GetMyTeams(c *gin.Context) {
	teams, err := h.teamService.GetTeamsForAdmin(c.Request.Context(), h.subject(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, teams)
}

// GetMyMembership GET /api/v1/team/me — current user's team_member record.
func (h *TeamHandler) GetMyMembership(c *gin.Context) {
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	member, err := h.teamService.GetMemberByUserID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

// ── Team detail (team_admin can rename their own team) ───────────────────────

type renameTeamRequest struct {
	Name string `json:"name" binding:"required"`
}

// UpdateTeam PUT /api/v1/team/:teamId
// Team_admin can rename their team. max_members is intentionally NOT modifiable
// here — only sys admin can change the cap (via /admin/teams/:id).
func (h *TeamHandler) UpdateTeam(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	var req renameTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	team, err := h.teamService.UpdateTeam(c.Request.Context(), teamID, service.UpdateTeamRequest{Name: req.Name})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

