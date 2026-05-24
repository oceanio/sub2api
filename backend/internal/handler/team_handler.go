package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
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

// ── Members ───────────────────────────────────────────────────────────────────

type createMemberRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Username string `json:"username"`
}

// CreateMember POST /api/v1/team/:teamId/members — create a NEW user account
// and add them to the team in a single transaction. sys admin's equivalent is
// AddMember (adds an existing user by id).
func (h *TeamHandler) CreateMember(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	var req createMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	member, err := h.teamService.CreateMemberUser(c.Request.Context(), service.CreateTeamMemberUserRequest{
		TeamID:     teamID,
		Email:      req.Email,
		Password:   req.Password,
		Username:   req.Username,
		OperatorID: h.subject(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

// GetMember GET /api/v1/team/:teamId/members/:memberID
func (h *TeamHandler) GetMember(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	member, err := h.teamService.GetMember(c.Request.Context(), teamID, memberID, h.subject(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

type updateTagsRequest struct {
	Tags []string `json:"tags"`
}

// UpdateMemberTags PUT /api/v1/team/:teamId/members/:memberID/tags
func (h *TeamHandler) UpdateMemberTags(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	var req updateTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	member, err := h.teamService.UpdateMemberTags(c.Request.Context(), teamID, memberID, h.subject(c), req.Tags)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

type setMemberStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// SetMemberStatus PUT /api/v1/team/:teamId/members/:memberID/status
func (h *TeamHandler) SetMemberStatus(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
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
	if err := h.teamService.SetMemberStatus(c.Request.Context(), teamID, memberID, h.subject(c), req.Status); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "status updated"})
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ResetMemberPassword PUT /api/v1/team/:teamId/members/:memberID/password
func (h *TeamHandler) ResetMemberPassword(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.teamService.ResetMemberPassword(c.Request.Context(), teamID, memberID, h.subject(c), req.NewPassword); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "password reset"})
}

// ListMemberAPIKeys GET /api/v1/team/:teamId/members/:memberID/api-keys
func (h *TeamHandler) ListMemberAPIKeys(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	keys, err := h.teamService.ListMemberAPIKeys(c.Request.Context(), teamID, memberID, h.subject(c))
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

// ListMemberSubscriptions GET /api/v1/team/:teamId/members/:memberID/subscriptions
func (h *TeamHandler) ListMemberSubscriptions(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid member id")
		return
	}
	subs, err := h.teamService.ListMemberSubscriptions(c.Request.Context(), teamID, memberID, h.subject(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, subs)
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

// ListPlans GET /api/v1/team/:teamId/plans
func (h *TeamHandler) ListPlans(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	plans, err := h.teamService.ListSubscriptionPlans(c.Request.Context(), teamID, h.subject(c))
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
	UserID int64 `json:"user_id" binding:"required"` // target member's user_id (NOT team_members.id)
	PlanID int64 `json:"plan_id" binding:"required"`
}

// PurchaseSubscription POST /api/v1/team/:teamId/subscriptions
func (h *TeamHandler) PurchaseSubscription(c *gin.Context) {
	teamID, ok := h.primaryTeamID(c)
	if !ok {
		return
	}
	var req purchaseSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	sub, err := h.teamService.PurchaseSubscriptionForMember(c.Request.Context(), teamID, req.UserID, req.PlanID, h.subject(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sub)
}
