package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// registerAdminTeamRoutes mounts all /admin/teams/* endpoints. Kept in a
// dedicated file so the team feature's footprint inside the admin route table
// stays an explicit, easy-to-locate block.
//
// Endpoints split into three layers:
//   - sys-admin-exclusive (list/create/delete/update/recharge/add-existing-user)
//   - team-scoped, shared with /team/:teamId/* via TeamScoped handler
func registerAdminTeamRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	teams := admin.Group("/teams")
	{
		teams.GET("", h.Admin.Team.ListTeams)
		teams.POST("", h.Admin.Team.CreateTeam)
		teams.PUT("/:id", h.Admin.Team.UpdateTeam)
		teams.DELETE("/:id", h.Admin.Team.DeleteTeam)
		teams.POST("/:id/recharge", h.Admin.Team.RechargeTeam)
		teams.POST("/:id/refund", h.Admin.Team.RefundTeam)
		teams.POST("/:id/members", h.Admin.Team.AddMember)
		teams.GET("/:id", h.TeamScoped.GetTeam)

		// Shared team-scoped endpoints: implementation lives in TeamScoped and
		// is shared with /team/:teamId/* (see routes/team.go).
		teams.GET("/:id/members", h.TeamScoped.ListMembers)
		teams.DELETE("/:id/members/:memberID", h.TeamScoped.RemoveMember)
		teams.PUT("/:id/members/:memberID/sub-quota", h.TeamScoped.UpdateMemberSubQuota)
		teams.POST("/:id/members/new", h.TeamScoped.CreateMember)
		teams.GET("/:id/members/:memberID", h.TeamScoped.GetMember)
		teams.PUT("/:id/members/:memberID/tags", h.TeamScoped.UpdateMemberTags)
		teams.PUT("/:id/members/:memberID/password", h.TeamScoped.ResetMemberPassword)
		teams.PUT("/:id/members/:memberID/status", h.TeamScoped.SetMemberStatus)
		teams.GET("/:id/members/:memberID/api-keys", h.TeamScoped.ListMemberAPIKeys)
		teams.GET("/:id/members/:memberID/subscriptions", h.TeamScoped.ListMemberSubscriptions)

		teams.GET("/:id/admins", h.TeamScoped.ListAdmins)
		teams.POST("/:id/admins", h.TeamScoped.AddAdmin)
		teams.DELETE("/:id/admins/:userID", h.TeamScoped.RemoveAdmin)

		teams.GET("/:id/usage", h.TeamScoped.ListUsage)
		teams.GET("/:id/usage/stats", h.TeamScoped.UsageStats)
		teams.GET("/:id/usage/trend", h.TeamScoped.UsageTrend)
		teams.GET("/:id/usage/model-stats", h.TeamScoped.UsageModelStats)
		teams.GET("/:id/usage/group-stats", h.TeamScoped.UsageGroupStats)
		teams.GET("/:id/usage/endpoint-stats", h.TeamScoped.UsageEndpointStats)
		teams.GET("/:id/usage/user-breakdown", h.TeamScoped.UsageUserBreakdown)
		teams.GET("/:id/subscriptions", h.TeamScoped.ListSubscriptions)
		teams.POST("/:id/subscriptions", h.TeamScoped.PurchaseSubscription)
		teams.GET("/:id/plans", h.TeamScoped.ListPlans)
		teams.GET("/:id/balance", h.TeamScoped.ListBalanceLogs)
		teams.PUT("/:id/tags", h.TeamScoped.UpdateAvailableTags)
	}
}
