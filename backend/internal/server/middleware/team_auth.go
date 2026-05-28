package middleware

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// TeamAuthMiddleware verifies the JWT-authenticated user is a team_admin of
// the team identified by URL :teamId. It must run after the JWT middleware.
type TeamAuthMiddleware gin.HandlerFunc

// NewTeamAuthMiddleware enforces:
//   - user is authenticated
//   - URL :teamId parses to a valid int64
//   - user is in team_admins for that teamId
//
// On success the resolved teamId is stored at "team_id" for handler access.
func NewTeamAuthMiddleware(teamService *service.TeamService) TeamAuthMiddleware {
	return TeamAuthMiddleware(func(c *gin.Context) {
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok {
			response.Unauthorized(c, "User not authenticated")
			c.Abort()
			return
		}
		teamIDParam := c.Param("teamId")
		if teamIDParam == "" {
			response.BadRequest(c, "team id required")
			c.Abort()
			return
		}
		teamID, err := strconv.ParseInt(teamIDParam, 10, 64)
		if err != nil || teamID <= 0 {
			response.BadRequest(c, "invalid team id")
			c.Abort()
			return
		}
		isAdmin, err := teamService.IsTeamAdmin(c.Request.Context(), teamID, subject.UserID)
		if err != nil {
			response.InternalError(c, err.Error())
			c.Abort()
			return
		}
		if !isAdmin {
			response.Forbidden(c, "Team admin permission required")
			c.Abort()
			return
		}
		c.Set("team_id", teamID)
		c.Next()
	})
}

// GetActiveTeamID returns the URL-resolved teamId from middleware-set context.
func GetActiveTeamID(c *gin.Context) (int64, bool) {
	val, exists := c.Get("team_id")
	if !exists {
		return 0, false
	}
	teamID, ok := val.(int64)
	return teamID, ok
}
