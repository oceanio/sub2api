package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ── Domain errors ─────────────────────────────────────────────────────────────

var (
	ErrTeamNotFound             = infraerrors.NotFound("TEAM_NOT_FOUND", "team not found")
	ErrTeamMemberNotFound       = infraerrors.NotFound("TEAM_MEMBER_NOT_FOUND", "team member not found")
	ErrTeamAdminNotFound        = infraerrors.NotFound("TEAM_ADMIN_NOT_FOUND", "team admin record not found")
	ErrTeamHasMembers           = infraerrors.BadRequest("TEAM_HAS_MEMBERS", "team still has active paying members, remove them first")
	ErrTeamInsufficientBalance  = infraerrors.BadRequest("TEAM_INSUFFICIENT_BALANCE", "team balance is insufficient")
	ErrTeamUserAlreadyMember    = infraerrors.Conflict("TEAM_USER_ALREADY_MEMBER", "user is already a paying member of a team")
	ErrTeamUserAlreadyAdmin     = infraerrors.Conflict("TEAM_USER_ALREADY_ADMIN", "user is already an admin of this team")
	ErrTeamLastAdmin            = infraerrors.BadRequest("TEAM_LAST_ADMIN", "cannot remove the last team_admin; assign another admin first")
	ErrTeamNotMember            = infraerrors.BadRequest("TEAM_NOT_MEMBER", "user is not a member of this team")
	ErrTeamAdminRequired        = infraerrors.Forbidden("TEAM_ADMIN_REQUIRED", "team admin permission required")
	ErrTeamBalanceLogNotFound   = infraerrors.NotFound("TEAM_BALANCE_LOG_NOT_FOUND", "team balance log not found")
	ErrTeamInitialAdminRequired = infraerrors.BadRequest("TEAM_INITIAL_ADMIN_REQUIRED", "initial team_admin is required when creating a team")
	ErrTeamEmailAlreadyExists   = infraerrors.Conflict("TEAM_EMAIL_ALREADY_EXISTS", "this email already exists on the platform; ask the system administrator to add the user to your team")
	ErrTeamMemberCapReached     = infraerrors.BadRequest("TEAM_MEMBER_CAP_REACHED", "this team has reached its member cap; ask the system administrator to raise it")
	ErrTeamSubscriptionsDisabled = infraerrors.Forbidden("TEAM_SUBSCRIPTIONS_DISABLED", "subscriptions feature is disabled for this team")
)

// ── Balance log type constants ─────────────────────────────────────────────────

const (
	TeamBalanceLogTypeRecharge             = "recharge"
	TeamBalanceLogTypeRefund               = "refund"
	TeamBalanceLogTypeSubscriptionPurchase = "subscription_purchase"
)

// ── Domain models ─────────────────────────────────────────────────────────────

type Team struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	Balance              float64   `json:"balance"`
	AvailableTags        []string  `json:"available_tags"`
	MaxMembers           int       `json:"max_members"` // 0 = unlimited
	SubscriptionsEnabled bool      `json:"subscriptions_enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`

	// Computed: populated by List() for admin display, zero otherwise.
	MemberCount int64 `json:"member_count"`
}

// TeamMember is a paying-membership record.
// v2: no Role field — admin role lives in TeamAdmin.
type TeamMember struct {
	ID           int64     `json:"id"`
	TeamID       int64     `json:"team_id"`
	UserID       int64     `json:"user_id"`
	SubQuota     float64   `json:"sub_quota"`
	SubQuotaUsed float64   `json:"sub_quota_used"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Loaded edges. We project User into a small struct with JSON tags because
	// service.User itself has no tags (handlers use dto.User for that), and
	// embedding it directly would serialise as PascalCase keys.
	User *TeamMemberUser `json:"user,omitempty"`
	Team *Team           `json:"team,omitempty"`

	// Computed fields populated by ListMembers (not stored on team_members table).
	ActiveSubscriptions int        `json:"active_subscriptions"`
	LastActiveAt        *time.Time `json:"last_active_at"`
	IsAdmin             bool       `json:"is_admin"` // populated by ListMembers: does this user have a team_admins row for this team?
}

// TeamAdmin is a management-role assignment.
type TeamAdmin struct {
	ID        int64     `json:"id"`
	TeamID    int64     `json:"team_id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`

	User *TeamMemberUser `json:"user,omitempty"`
}

// TeamMemberUser is the user snapshot embedded in TeamMember JSON responses.
type TeamMemberUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

type TeamBalanceLog struct {
	ID           int64     `json:"id"`
	TeamID       int64     `json:"team_id"`
	Type         string    `json:"type"`
	Amount       float64   `json:"amount"`
	OperatorID   int64     `json:"operator_id"`
	TargetUserID *int64    `json:"target_user_id,omitempty"`
	Note         *string   `json:"note,omitempty"`
	CreatedAt    time.Time `json:"created_at"`

	Operator   *TeamMemberUser `json:"operator,omitempty"`
	TargetUser *TeamMemberUser `json:"target_user,omitempty"`
}

// ── Repository interfaces ──────────────────────────────────────────────────────

type TeamRepository interface {
	Create(ctx context.Context, team *Team) error
	// CreateWithInitialState atomically writes teams + team_admins
	// (and optionally team_members if alsoAddAsMember is true, and a recharge
	// balance log if initialBalance > 0). Single ent transaction.
	CreateWithInitialState(ctx context.Context, team *Team, initialAdminUserID int64, initialBalance float64, maxMembers int, alsoAddAsMember bool, subscriptionsEnabled *bool, operatorID int64) error
	GetByID(ctx context.Context, id int64) (*Team, error)
	// ListByIDs batch-loads teams with populated MemberCount. Used for sidebar
	// and selector loads to avoid the GetTeamsForAdmin N+1 pattern.
	ListByIDs(ctx context.Context, ids []int64) ([]Team, error)
	Update(ctx context.Context, team *Team) error
	SoftDelete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams) ([]Team, *pagination.PaginationResult, error)
	// AddBalance atomically adds amount to teams.balance (negative to deduct). Returns the new balance.
	AddBalance(ctx context.Context, id int64, amount float64) (float64, error)
	CountActiveMembers(ctx context.Context, id int64) (int64, error)
	UpdateAvailableTags(ctx context.Context, id int64, tags []string) error
	CountActiveMembersByTeamIDs(ctx context.Context, ids []int64) (map[int64]int64, error)
}

type TeamMemberRepository interface {
	Create(ctx context.Context, member *TeamMember) error
	GetByID(ctx context.Context, id int64) (*TeamMember, error)
	GetByUserID(ctx context.Context, userID int64) (*TeamMember, error)
	GetByTeamAndUserID(ctx context.Context, teamID, userID int64) (*TeamMember, error)
	// ListMemberUserIDs returns active member user_ids for teamID. If filter is
	// non-empty, the result is restricted to user_ids in that set — used by bulk
	// purchase to validate a request set with one IN query, or to snapshot the
	// full team for "all members" purchases when filter is nil.
	ListMemberUserIDs(ctx context.Context, teamID int64, filter []int64) ([]int64, error)
	UpdateTags(ctx context.Context, id int64, tags []string) error
	SoftDelete(ctx context.Context, id int64) error
	ListByTeamID(ctx context.Context, teamID int64, params pagination.PaginationParams) ([]TeamMember, *pagination.PaginationResult, error)
	ListByTeamIDFiltered(ctx context.Context, teamID int64, filters TeamMemberListFilters, params pagination.PaginationParams) ([]TeamMember, *pagination.PaginationResult, error)
	UpdateSubQuota(ctx context.Context, id int64, subQuota float64) error
	IncrementSubQuotaUsed(ctx context.Context, id int64, cost float64) error
	DisableAPIKeysByTeamID(ctx context.Context, userID, teamID int64) error
	GetActivityStats(ctx context.Context, teamID int64, userIDs []int64) (map[int64]MemberActivityStats, error)
}

// TeamAdminRepository handles the team_admins table (management role assignments).
type TeamAdminRepository interface {
	Create(ctx context.Context, teamID, userID int64) (*TeamAdmin, error)
	SoftDelete(ctx context.Context, teamID, userID int64) error
	ExistsByTeamAndUser(ctx context.Context, teamID, userID int64) (bool, error)
	ListByTeamID(ctx context.Context, teamID int64) ([]TeamAdmin, error)
	ListTeamIDsByUserID(ctx context.Context, userID int64) ([]int64, error)
	CountActiveByTeamID(ctx context.Context, teamID int64) (int64, error)
	// IsAdminMap returns map[userID]bool indicating whether each user is an admin of teamID.
	IsAdminMap(ctx context.Context, teamID int64, userIDs []int64) (map[int64]bool, error)
}

// MemberActivityStats holds computed fields for the team member list view.
type MemberActivityStats struct {
	ActiveSubscriptions int
	LastActiveAt        *time.Time
}

type TeamBalanceLogRepository interface {
	Create(ctx context.Context, log *TeamBalanceLog) error
	ListByTeamID(ctx context.Context, teamID int64, params pagination.PaginationParams) ([]TeamBalanceLog, *pagination.PaginationResult, error)
}

// ── Request/response types ─────────────────────────────────────────────────────

type CreateTeamRequest struct {
	Name                 string
	InitialAdminUserID   int64   // required
	InitialBalance       float64 // optional (0 = no recharge log)
	MaxMembers           int     // optional (0 = unlimited)
	AlsoAddAsMember      bool    // optional: also add the initial admin as paying member
	SubscriptionsEnabled *bool   // optional; nil = use ent default (true)
	OperatorID           int64   // sys admin user id
}

type UpdateTeamRequest struct {
	Name                 string
	MaxMembers           *int  // sys admin only; pointer so absence means "leave unchanged"
	SubscriptionsEnabled *bool // sys admin only; pointer so absence means "leave unchanged"

	// GroupRates 与 GroupRPMOverrides 是 admin 在「分组管理」tab 编辑的两个
	// 平行 map。语义与 user_group 同：
	//   nil 整个字段     → 不动 (不做任何 sync)
	//   非 nil 但条目空  → 等同于不动 (sync 是 no-op)
	//   条目 value nil   → 清空该列；若 rate+rpm 都为 NULL 则整行 DELETE
	//   条目 value 非 nil → upsert
	GroupRates        map[int64]*float64 // group_id → rate_multiplier override
	GroupRPMOverrides map[int64]*int     // group_id → rpm override (0 = exempt)
}

type RechargeTeamRequest struct {
	Amount     float64
	OperatorID int64
	Note       string
}

// AddTeamMemberRequest adds an existing user as a paying member.
type AddTeamMemberRequest struct {
	TeamID  int64
	UserID  int64
	ByAdmin int64
}

type CreateTeamMemberUserRequest struct {
	TeamID     int64
	Email      string
	Password   string
	Username   string
	OperatorID int64
}

type UpdateTeamMemberSubQuotaRequest struct {
	TeamID     int64
	MemberID   int64
	SubQuota   float64
	OperatorID int64
}

type RemoveTeamMemberRequest struct {
	TeamID     int64
	MemberID   int64
	OperatorID int64
}

// AddTeamAdminRequest grants admin role to a user on a team.
type AddTeamAdminRequest struct {
	TeamID     int64
	UserID     int64
	OperatorID int64
}

// RemoveTeamAdminRequest revokes admin role.
type RemoveTeamAdminRequest struct {
	TeamID     int64
	UserID     int64
	OperatorID int64
}

type TeamListFilters struct {
	Search string
}

type TeamMemberListFilters struct {
	Search string
	Tags   []string
}
