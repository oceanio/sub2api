package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterTeamRoutes registers team management routes for team_admin users.
//
// URL pattern: /team/:teamId/* — every page-data endpoint carries the team id
// so the same user can manage multiple teams via a selector.
func RegisterTeamRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	teamAuth middleware.TeamAuthMiddleware,
	settingService *service.SettingService,
) {
	// JWT-only paths: accessible to any authenticated user (member or non-member).
	jwtOnly := v1.Group("")
	jwtOnly.Use(gin.HandlerFunc(jwtAuth))
	jwtOnly.Use(middleware.BackendModeUserGuard(settingService))
	jwtOnly.GET("/team/me", h.Team.GetMyMembership)
	// List my managed teams — used by the team selector.
	jwtOnly.GET("/team/teams", h.Team.GetMyTeams)

	// All /team/:teamId/* routes require team_admin membership for that teamId.
	scoped := v1.Group("/team/:teamId")
	scoped.Use(gin.HandlerFunc(jwtAuth))
	scoped.Use(middleware.BackendModeUserGuard(settingService))
	scoped.Use(gin.HandlerFunc(teamAuth))
	{
		// Shared endpoints (also routed under /admin/teams/:id/* in routes/admin.go).
		scoped.GET("", h.TeamScoped.GetTeam)
		scoped.PUT("/tags", h.TeamScoped.UpdateAvailableTags)

		// team_admin can rename their own team (sys admin uses PUT /admin/teams/:id
		// for full edits including max_members).
		scoped.PUT("", h.Team.UpdateTeam)

		members := scoped.Group("/members")
		{
			members.GET("", h.TeamScoped.ListMembers)
			members.DELETE("/:memberID", h.TeamScoped.RemoveMember)
			members.PUT("/:memberID/sub-quota", h.TeamScoped.UpdateMemberSubQuota)

			// Shared (also routed under /admin/teams/:id/members/* in admin_teams.go).
			members.POST("", h.TeamScoped.CreateMember)
			members.POST("/batch", h.TeamScoped.BulkCreateMembers)
			members.GET("/:memberID", h.TeamScoped.GetMember)
			members.PUT("/:memberID/tags", h.TeamScoped.UpdateMemberTags)
			members.PUT("/:memberID/password", h.TeamScoped.ResetMemberPassword)
			members.PUT("/:memberID/status", h.TeamScoped.SetMemberStatus)
			members.GET("/:memberID/api-keys", h.TeamScoped.ListMemberAPIKeys)
			members.GET("/:memberID/subscriptions", h.TeamScoped.ListMemberSubscriptions)
		}

		// Admin role management (team_admin can add/remove other admins on this team).
		admins := scoped.Group("/admins")
		{
			admins.GET("", h.TeamScoped.ListAdmins)
			admins.POST("", h.TeamScoped.AddAdmin)
			admins.DELETE("/:userID", h.TeamScoped.RemoveAdmin)
		}

		scoped.GET("/usage", h.TeamScoped.ListUsage)
		scoped.GET("/usage/stats", h.TeamScoped.UsageStats)
		scoped.GET("/usage/trend", h.TeamScoped.UsageTrend)
		scoped.GET("/usage/model-stats", h.TeamScoped.UsageModelStats)
		scoped.GET("/usage/group-stats", h.TeamScoped.UsageGroupStats)
		scoped.GET("/usage/endpoint-stats", h.TeamScoped.UsageEndpointStats)
		scoped.GET("/usage/user-breakdown", h.TeamScoped.UsageUserBreakdown)
		scoped.GET("/balance", h.TeamScoped.ListBalanceLogs)
		scoped.GET("/subscriptions", h.TeamScoped.ListSubscriptions)

		scoped.POST("/subscriptions", h.TeamScoped.PurchaseSubscription)
		scoped.GET("/plans", h.TeamScoped.ListPlans)
	}
}
