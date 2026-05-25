package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminTeamHandler hosts the sys-admin-only team endpoints. Shared endpoints
// that also serve team_admin live in handler/teamscoped.
type AdminTeamHandler struct {
	teamService *service.TeamService
}

func NewAdminTeamHandler(teamService *service.TeamService) *AdminTeamHandler {
	return &AdminTeamHandler{teamService: teamService}
}

// ── DTOs ──────────────────────────────────────────────────────────────────────

type createTeamRequest struct {
	Name                 string  `json:"name" binding:"required"`
	InitialAdminUserID   int64   `json:"initial_admin_user_id" binding:"required"`
	InitialBalance       float64 `json:"initial_balance" binding:"gte=0"`
	MaxMembers           int     `json:"max_members" binding:"gte=0"`
	AlsoAddAsMember      bool    `json:"also_add_as_member"`
	SubscriptionsEnabled *bool   `json:"subscriptions_enabled"`
}

type updateTeamRequest struct {
	Name                 string `json:"name"` // PATCH semantics: 空串表示不动 (service 内 if req.Name != "" 才赋值)
	MaxMembers           *int   `json:"max_members" binding:"omitempty,gte=0"`
	SubscriptionsEnabled *bool  `json:"subscriptions_enabled"`

	// 团队×分组 倍率/RPM 覆盖（admin 在「分组管理」tab 编辑）。
	// 与 user_group 的 group_rates 形状对称：缺席 = 不动；value nil = 清空该列；
	// 两列都被清空时整行 DELETE。
	GroupRates        map[int64]*float64 `json:"group_rates"`
	GroupRPMOverrides map[int64]*int     `json:"group_rpm_overrides"`
}

type rechargeTeamRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Note   string  `json:"note"`
}

type addMemberRequest struct {
	UserID int64 `json:"user_id" binding:"required"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// ListTeams GET /api/v1/admin/teams
func (h *AdminTeamHandler) ListTeams(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}

	teams, result, err := h.teamService.ListTeams(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, teams, result.Total, page, pageSize)
}

// CreateTeam POST /api/v1/admin/teams
func (h *AdminTeamHandler) CreateTeam(c *gin.Context) {
	var req createTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	team, err := h.teamService.CreateTeam(c.Request.Context(), service.CreateTeamRequest{
		Name:                 req.Name,
		InitialAdminUserID:   req.InitialAdminUserID,
		InitialBalance:       req.InitialBalance,
		MaxMembers:           req.MaxMembers,
		AlsoAddAsMember:      req.AlsoAddAsMember,
		SubscriptionsEnabled: req.SubscriptionsEnabled,
		OperatorID:           subject.UserID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

// UpdateTeam PUT /api/v1/admin/teams/:id
func (h *AdminTeamHandler) UpdateTeam(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id")
		return
	}
	var req updateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	team, err := h.teamService.UpdateTeam(c.Request.Context(), id, service.UpdateTeamRequest{
		Name:                 req.Name,
		MaxMembers:           req.MaxMembers,
		SubscriptionsEnabled: req.SubscriptionsEnabled,
		GroupRates:           req.GroupRates,
		GroupRPMOverrides:    req.GroupRPMOverrides,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

// DeleteTeam DELETE /api/v1/admin/teams/:id
func (h *AdminTeamHandler) DeleteTeam(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id")
		return
	}
	if err := h.teamService.DeleteTeam(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "team deleted"})
}

// GetGroupRateConfig GET /api/v1/admin/teams/:id/group-rates
// 返回团队×分组的倍率/RPM 覆盖两个 map，给「分组管理」编辑页用。仅 sys admin。
func (h *AdminTeamHandler) GetGroupRateConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id")
		return
	}
	rates, rpms, err := h.teamService.GetTeamGroupRateConfig(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 显式空 map 而非 nil，方便前端不区分 "尚未加载" / "无覆盖"。
	if rates == nil {
		rates = map[int64]float64{}
	}
	if rpms == nil {
		rpms = map[int64]int{}
	}
	response.Success(c, gin.H{
		"group_rates":         rates,
		"group_rpm_overrides": rpms,
	})
}

// RechargeTeam POST /api/v1/admin/teams/:id/recharge
func (h *AdminTeamHandler) RechargeTeam(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id")
		return
	}
	var req rechargeTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	team, err := h.teamService.RechargeTeam(c.Request.Context(), id, service.RechargeTeamRequest{
		Amount:     req.Amount,
		OperatorID: subject.UserID,
		Note:       req.Note,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

// RefundTeam POST /api/v1/admin/teams/:id/refund
func (h *AdminTeamHandler) RefundTeam(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id")
		return
	}
	var req rechargeTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	team, err := h.teamService.RefundTeam(c.Request.Context(), id, service.RechargeTeamRequest{
		Amount:     req.Amount,
		OperatorID: subject.UserID,
		Note:       req.Note,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, team)
}

// AddMember POST /api/v1/admin/teams/:id/members
// Adds an existing user as a paying team member (sys admin only).
// team_admin uses POST /team/:teamId/members to create a NEW user and add.
func (h *AdminTeamHandler) AddMember(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id")
		return
	}
	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	member, err := h.teamService.AddMemberToTeam(c.Request.Context(), service.AddTeamMemberRequest{
		TeamID:  id,
		UserID:  req.UserID,
		ByAdmin: subject.UserID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}
