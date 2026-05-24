package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// TeamUsageRepository is the team-scoped slice of UsageLogRepository. Splitting
// it out lets the team feature keep its repo-layer code in a separate file
// (repository/usage_log_team_repo.go) without polluting the main interface.
type TeamUsageRepository interface {
	ListTeamUsageLogs(ctx context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error)
	GetTeamUsageStats(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error)
	GetTeamUsageTrend(ctx context.Context, filters usagestats.UsageLogFilters, granularity string) ([]usagestats.TrendDataPoint, error)
	GetTeamModelStats(ctx context.Context, filters usagestats.UsageLogFilters, source string) ([]usagestats.ModelStat, error)
	GetTeamGroupStats(ctx context.Context, filters usagestats.UsageLogFilters) ([]usagestats.GroupStat, error)
	GetTeamEndpointStats(ctx context.Context, filters usagestats.UsageLogFilters) ([]usagestats.EndpointStat, []usagestats.EndpointStat, error)
	GetTeamUserBreakdown(ctx context.Context, filters usagestats.UsageLogFilters, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error)
}

// TeamService handles all team management operations.
type TeamService struct {
	teamRepo             TeamRepository
	memberRepo           TeamMemberRepository
	adminRepo            TeamAdminRepository
	balanceLogRepo       TeamBalanceLogRepository
	userRepo             UserRepository
	apiKeyRepo           APIKeyRepository
	userSubRepo          UserSubscriptionRepository
	subService           *SubscriptionService
	paymentConfigService *PaymentConfigService
	teamUsageRepo        TeamUsageRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	entClient            *dbent.Client
}

func NewTeamService(
	teamRepo TeamRepository,
	memberRepo TeamMemberRepository,
	adminRepo TeamAdminRepository,
	balanceLogRepo TeamBalanceLogRepository,
	userRepo UserRepository,
	apiKeyRepo APIKeyRepository,
	userSubRepo UserSubscriptionRepository,
	subService *SubscriptionService,
	paymentConfigService *PaymentConfigService,
	teamUsageRepo TeamUsageRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	entClient *dbent.Client,
) *TeamService {
	return &TeamService{
		teamRepo:             teamRepo,
		memberRepo:           memberRepo,
		adminRepo:            adminRepo,
		balanceLogRepo:       balanceLogRepo,
		userRepo:             userRepo,
		apiKeyRepo:           apiKeyRepo,
		userSubRepo:          userSubRepo,
		subService:           subService,
		paymentConfigService: paymentConfigService,
		teamUsageRepo:        teamUsageRepo,
		authCacheInvalidator: authCacheInvalidator,
		entClient:            entClient,
	}
}

// invalidateAuthCacheForUser is a nil-safe wrapper.
func (s *TeamService) invalidateAuthCacheForUser(ctx context.Context, userID int64) {
	if s.authCacheInvalidator == nil || userID <= 0 {
		return
	}
	s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
}

// checkMemberCap returns ErrTeamMemberCapReached if the team has reached its
// max_members cap. cap=0 means unlimited.
func (s *TeamService) checkMemberCap(ctx context.Context, teamID int64) error {
	t, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if t.MaxMembers <= 0 {
		return nil
	}
	count, err := s.teamRepo.CountActiveMembers(ctx, teamID)
	if err != nil {
		return err
	}
	if count >= int64(t.MaxMembers) {
		return ErrTeamMemberCapReached
	}
	return nil
}

// RequireTeamAdmin is a public alias of requireTeamAdmin for handler use.
func (s *TeamService) RequireTeamAdmin(ctx context.Context, teamID, userID int64) error {
	return s.requireTeamAdmin(ctx, teamID, userID)
}

// Team usage read paths all go through the team-only repo file
// (repository/usage_log_team_repo.go). AccountID inside filters is never
// honored — team usage must not depend on upstream-account data.

func (s *TeamService) ListUsageLogs(ctx context.Context, filters usagestats.UsageLogFilters, params pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return s.teamUsageRepo.ListTeamUsageLogs(ctx, params, filters)
}

// GetUsageStats returns aggregate totals for the team usage summary cards.
func (s *TeamService) GetUsageStats(ctx context.Context, filters usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return s.teamUsageRepo.GetTeamUsageStats(ctx, filters)
}

// GetTeamUsageTrend returns the usage trend points for the team.
func (s *TeamService) GetTeamUsageTrend(ctx context.Context, filters usagestats.UsageLogFilters, granularity string) ([]usagestats.TrendDataPoint, error) {
	return s.teamUsageRepo.GetTeamUsageTrend(ctx, filters, granularity)
}

// GetTeamModelStats returns per-model breakdown for the team, with the given
// model dimension (requested / upstream / mapping).
func (s *TeamService) GetTeamModelStats(ctx context.Context, filters usagestats.UsageLogFilters, modelSource string) ([]usagestats.ModelStat, error) {
	return s.teamUsageRepo.GetTeamModelStats(ctx, filters, modelSource)
}

// GetTeamGroupStats returns per-group breakdown for the team.
func (s *TeamService) GetTeamGroupStats(ctx context.Context, filters usagestats.UsageLogFilters) ([]usagestats.GroupStat, error) {
	return s.teamUsageRepo.GetTeamGroupStats(ctx, filters)
}

// GetTeamUserBreakdown returns per-user breakdown within a dimension. The
// caller is expected to pre-fill dim.* with their own filters; the team scope
// (dim.TeamID and AccountID=0) is enforced by the repo layer.
func (s *TeamService) GetTeamUserBreakdown(ctx context.Context, filters usagestats.UsageLogFilters, dim usagestats.UserBreakdownDimension, limit int) ([]usagestats.UserBreakdownItem, error) {
	if filters.UserID > 0 && dim.UserID == 0 {
		dim.UserID = filters.UserID
	}
	if filters.APIKeyID > 0 && dim.APIKeyID == 0 {
		dim.APIKeyID = filters.APIKeyID
	}
	if filters.RequestType != nil && dim.RequestType == nil {
		dim.RequestType = filters.RequestType
	}
	if filters.Stream != nil && dim.Stream == nil {
		dim.Stream = filters.Stream
	}
	if filters.BillingType != nil && dim.BillingType == nil {
		dim.BillingType = filters.BillingType
	}
	dim.AccountID = 0
	return s.teamUsageRepo.GetTeamUserBreakdown(ctx, filters, dim, limit)
}

// GetTeamEndpointStats returns inbound endpoint and inbound->upstream-path
// breakdowns. The upstream-only column is intentionally omitted from the team
// view to avoid exposing upstream account identifiers.
func (s *TeamService) GetTeamEndpointStats(ctx context.Context, filters usagestats.UsageLogFilters) ([]usagestats.EndpointStat, []usagestats.EndpointStat, error) {
	return s.teamUsageRepo.GetTeamEndpointStats(ctx, filters)
}

// ── Admin operations ───────────────────────────────────────────────────────────

func (s *TeamService) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	if req.InitialAdminUserID <= 0 {
		return nil, ErrTeamInitialAdminRequired
	}
	if req.InitialBalance < 0 {
		return nil, fmt.Errorf("initial balance must be non-negative")
	}
	if req.MaxMembers < 0 {
		return nil, fmt.Errorf("max_members must be non-negative")
	}
	team := &Team{Name: req.Name}
	if err := s.teamRepo.CreateWithInitialState(ctx, team, req.InitialAdminUserID, req.InitialBalance, req.MaxMembers, req.AlsoAddAsMember, req.OperatorID); err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return team, nil
}

func (s *TeamService) GetTeam(ctx context.Context, id int64) (*Team, error) {
	return s.teamRepo.GetByID(ctx, id)
}

func (s *TeamService) UpdateTeam(ctx context.Context, id int64, req UpdateTeamRequest) (*Team, error) {
	// Load existing to preserve any field the caller didn't pass.
	existing, err := s.teamRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.MaxMembers != nil {
		if *req.MaxMembers < 0 {
			return nil, fmt.Errorf("max_members must be >= 0")
		}
		existing.MaxMembers = *req.MaxMembers
	}
	if err := s.teamRepo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update team: %w", err)
	}
	return s.teamRepo.GetByID(ctx, id)
}

func (s *TeamService) DeleteTeam(ctx context.Context, id int64) error {
	count, err := s.teamRepo.CountActiveMembers(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrTeamHasMembers
	}
	return s.teamRepo.SoftDelete(ctx, id)
}

func (s *TeamService) ListTeams(ctx context.Context, params pagination.PaginationParams) ([]Team, *pagination.PaginationResult, error) {
	return s.teamRepo.List(ctx, params)
}

// RechargeTeam adds balance to a team and writes a balance log entry.
func (s *TeamService) RechargeTeam(ctx context.Context, teamID int64, req RechargeTeamRequest) (*Team, error) {
	if req.Amount <= 0 {
		return nil, fmt.Errorf("recharge amount must be positive")
	}
	newBalance, err := s.teamRepo.AddBalance(ctx, teamID, req.Amount)
	if err != nil {
		return nil, fmt.Errorf("recharge team: %w", err)
	}
	note := req.Note
	log := &TeamBalanceLog{
		TeamID:     teamID,
		Type:       TeamBalanceLogTypeRecharge,
		Amount:     req.Amount,
		OperatorID: req.OperatorID,
		Note:       &note,
	}
	if err := s.balanceLogRepo.Create(ctx, log); err != nil {
		return nil, fmt.Errorf("create balance log: %w", err)
	}
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	team.Balance = newBalance
	return team, nil
}

// RefundTeam deducts balance from a team and writes a balance log entry of
// type=refund. Rejects overdraft (team.balance < amount) — the personal user
// refund flow uses the same client-side guard plus a server-side check.
func (s *TeamService) RefundTeam(ctx context.Context, teamID int64, req RechargeTeamRequest) (*Team, error) {
	if req.Amount <= 0 {
		return nil, fmt.Errorf("refund amount must be positive")
	}
	current, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if current.Balance < req.Amount {
		return nil, ErrTeamInsufficientBalance
	}
	newBalance, err := s.teamRepo.AddBalance(ctx, teamID, -req.Amount)
	if err != nil {
		return nil, fmt.Errorf("refund team: %w", err)
	}
	note := req.Note
	log := &TeamBalanceLog{
		TeamID:     teamID,
		Type:       TeamBalanceLogTypeRefund,
		Amount:     -req.Amount,
		OperatorID: req.OperatorID,
		Note:       &note,
	}
	if err := s.balanceLogRepo.Create(ctx, log); err != nil {
		return nil, fmt.Errorf("create balance log: %w", err)
	}
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	team.Balance = newBalance
	return team, nil
}

// AddMemberToTeam adds an existing user as a paying member (sys admin only).
// Cap check applies: sys admin must raise max_members first if at limit.
func (s *TeamService) AddMemberToTeam(ctx context.Context, req AddTeamMemberRequest) (*TeamMember, error) {
	if err := s.checkMemberCap(ctx, req.TeamID); err != nil {
		return nil, err
	}
	member := &TeamMember{
		TeamID: req.TeamID,
		UserID: req.UserID,
	}
	if err := s.memberRepo.Create(ctx, member); err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}
	return s.memberRepo.GetByID(ctx, member.ID)
}

// ── Admin role operations (team_admins table) ─────────────────────────────────

// AddTeamAdmin grants admin role to a user on a team.
// Both sys admin and existing team_admin can call this.
func (s *TeamService) AddTeamAdmin(ctx context.Context, req AddTeamAdminRequest) (*TeamAdmin, error) {
	ta, err := s.adminRepo.Create(ctx, req.TeamID, req.UserID)
	if err != nil {
		return nil, err
	}
	return ta, nil
}

// RemoveTeamAdmin revokes admin role. Enforces last-admin guard.
func (s *TeamService) RemoveTeamAdmin(ctx context.Context, req RemoveTeamAdminRequest) error {
	count, err := s.adminRepo.CountActiveByTeamID(ctx, req.TeamID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrTeamLastAdmin
	}
	return s.adminRepo.SoftDelete(ctx, req.TeamID, req.UserID)
}

// AdminRemoveAdminUnsafe revokes admin role without the last-admin guard.
// Sys-admin-only path for team-deletion flow (spec §6.5).
func (s *TeamService) AdminRemoveAdminUnsafe(ctx context.Context, teamID, userID int64) error {
	return s.adminRepo.SoftDelete(ctx, teamID, userID)
}

// ListTeamAdmins lists all admins of a team.
func (s *TeamService) ListTeamAdmins(ctx context.Context, teamID int64) ([]TeamAdmin, error) {
	return s.adminRepo.ListByTeamID(ctx, teamID)
}

// ── team_admin operations ──────────────────────────────────────────────────────

// CreateMemberUser creates a new user account and adds them to the team in a
// single transaction. Refuses if the email already exists on the platform.
func (s *TeamService) CreateMemberUser(ctx context.Context, req CreateTeamMemberUserRequest) (*TeamMember, error) {
	if err := s.requireTeamAdmin(ctx, req.TeamID, req.OperatorID); err != nil {
		return nil, err
	}
	if err := s.checkMemberCap(ctx, req.TeamID); err != nil {
		return nil, err
	}

	user := &User{
		Email:    req.Email,
		Username: req.Username,
		Role:     RoleUser,
		Status:   StatusActive,
	}
	if err := user.SetPassword(req.Password); err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	if err := s.userRepo.Create(txCtx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return nil, ErrTeamEmailAlreadyExists
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	member := &TeamMember{
		TeamID: req.TeamID,
		UserID: user.ID,
	}
	if err := s.memberRepo.Create(txCtx, member); err != nil {
		return nil, fmt.Errorf("create team member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	member.User = &TeamMemberUser{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
		Status:   user.Status,
	}
	return member, nil
}

// UpdateMemberSubQuota sets the per-member spend cap.
func (s *TeamService) UpdateMemberSubQuota(ctx context.Context, req UpdateTeamMemberSubQuotaRequest) (*TeamMember, error) {
	if err := s.requireTeamAdmin(ctx, req.TeamID, req.OperatorID); err != nil {
		return nil, err
	}
	member, err := s.memberRepo.GetByID(ctx, req.MemberID)
	if err != nil {
		return nil, err
	}
	if member.TeamID != req.TeamID {
		return nil, ErrTeamMemberNotFound
	}
	if err := s.memberRepo.UpdateSubQuota(ctx, req.MemberID, req.SubQuota); err != nil {
		return nil, fmt.Errorf("update sub quota: %w", err)
	}
	member.SubQuota = req.SubQuota
	s.invalidateAuthCacheForUser(ctx, member.UserID)
	return member, nil
}

// RemoveMember removes a paying-member record. Admin role (if any) is unaffected.
func (s *TeamService) RemoveMember(ctx context.Context, req RemoveTeamMemberRequest) error {
	if err := s.requireTeamAdmin(ctx, req.TeamID, req.OperatorID); err != nil {
		return err
	}
	member, err := s.memberRepo.GetByID(ctx, req.MemberID)
	if err != nil {
		return err
	}
	if member.TeamID != req.TeamID {
		return ErrTeamMemberNotFound
	}
	if err := s.memberRepo.DisableAPIKeysByTeamID(ctx, member.UserID, req.TeamID); err != nil {
		return fmt.Errorf("disable team api keys: %w", err)
	}
	if err := s.memberRepo.SoftDelete(ctx, req.MemberID); err != nil {
		return err
	}
	s.invalidateAuthCacheForUser(ctx, member.UserID)
	return nil
}

// ListMembers returns paginated members for a team with stats.
func (s *TeamService) ListMembers(ctx context.Context, teamID int64, operatorID int64, params pagination.PaginationParams) ([]TeamMember, *pagination.PaginationResult, error) {
	return s.ListMembersFiltered(ctx, teamID, operatorID, nil, params)
}

// ListMembersFiltered returns paginated members for a team, filtered by tags (OR-match).
func (s *TeamService) ListMembersFiltered(ctx context.Context, teamID, operatorID int64, tags []string, params pagination.PaginationParams) ([]TeamMember, *pagination.PaginationResult, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, nil, err
	}
	return s.listMembersScoped(ctx, teamID, tags, params)
}

func (s *TeamService) listMembersScoped(ctx context.Context, teamID int64, tags []string, params pagination.PaginationParams) ([]TeamMember, *pagination.PaginationResult, error) {
	var (
		members []TeamMember
		result  *pagination.PaginationResult
		err     error
	)
	if len(tags) > 0 {
		members, result, err = s.memberRepo.ListByTeamIDFiltered(ctx, teamID, tags, params)
	} else {
		members, result, err = s.memberRepo.ListByTeamID(ctx, teamID, params)
	}
	if err != nil {
		return nil, nil, err
	}
	// Stats (2 batched queries) and admin-flag (1 batched query) are independent;
	// run in parallel to roughly halve the enrichment latency.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.enrichMembersWithStats(ctx, teamID, members) }()
	go func() { defer wg.Done(); s.enrichMembersWithAdminFlag(ctx, teamID, members) }()
	wg.Wait()
	return members, result, nil
}

// ListMemberUserIDs returns just the user_id list for a team.
func (s *TeamService) ListMemberUserIDs(ctx context.Context, teamID, operatorID int64) ([]int64, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	return s.listMemberUserIDsScoped(ctx, teamID)
}

func (s *TeamService) listMemberUserIDsScoped(ctx context.Context, teamID int64) ([]int64, error) {
	members, _, err := s.memberRepo.ListByTeamID(ctx, teamID, pagination.PaginationParams{Page: 1, PageSize: 5000})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	for i := range members {
		ids = append(ids, members[i].UserID)
	}
	return ids, nil
}

func (s *TeamService) enrichMembersWithStats(ctx context.Context, teamID int64, members []TeamMember) {
	if len(members) == 0 || s.memberRepo == nil {
		return
	}
	userIDs := make([]int64, 0, len(members))
	for i := range members {
		userIDs = append(userIDs, members[i].UserID)
	}
	stats, err := s.memberRepo.GetActivityStats(ctx, teamID, userIDs)
	if err != nil || stats == nil {
		return
	}
	for i := range members {
		if v, ok := stats[members[i].UserID]; ok {
			members[i].ActiveSubscriptions = v.ActiveSubscriptions
			members[i].LastActiveAt = v.LastActiveAt
		}
	}
}

func (s *TeamService) enrichMembersWithAdminFlag(ctx context.Context, teamID int64, members []TeamMember) {
	if len(members) == 0 || s.adminRepo == nil {
		return
	}
	userIDs := make([]int64, 0, len(members))
	for i := range members {
		userIDs = append(userIDs, members[i].UserID)
	}
	adminMap, err := s.adminRepo.IsAdminMap(ctx, teamID, userIDs)
	if err != nil {
		return
	}
	for i := range members {
		members[i].IsAdmin = adminMap[members[i].UserID]
	}
}

// GetMember returns a single member.
func (s *TeamService) GetMember(ctx context.Context, teamID, memberID, operatorID int64) (*TeamMember, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	member, err := s.memberRepo.GetByID(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if member.TeamID != teamID {
		return nil, ErrTeamMemberNotFound
	}
	return member, nil
}

// ListBalanceLogs returns paginated balance logs for a team (team_admin path).
func (s *TeamService) ListBalanceLogs(ctx context.Context, teamID, operatorID int64, params pagination.PaginationParams) ([]TeamBalanceLog, *pagination.PaginationResult, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, nil, err
	}
	return s.balanceLogRepo.ListByTeamID(ctx, teamID, params)
}

// GetTeamsForAdmin returns all teams where the user is in team_admins table.
// Uses batched ListByIDs to avoid N+1.
func (s *TeamService) GetTeamsForAdmin(ctx context.Context, userID int64) ([]Team, error) {
	teamIDs, err := s.adminRepo.ListTeamIDsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(teamIDs) == 0 {
		return nil, nil
	}
	return s.teamRepo.ListByIDs(ctx, teamIDs)
}

// IsTeamMember reports whether userID is an active paying member of teamID.
func (s *TeamService) IsTeamMember(ctx context.Context, teamID, userID int64) (bool, error) {
	_, err := s.memberRepo.GetByTeamAndUserID(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, ErrTeamMemberNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsTeamAdmin returns true iff team_admins(teamID, userID) row exists.
func (s *TeamService) IsTeamAdmin(ctx context.Context, teamID, userID int64) (bool, error) {
	return s.adminRepo.ExistsByTeamAndUser(ctx, teamID, userID)
}

// GetMemberByUserID returns the (paying) TeamMember record for a user if any.
func (s *TeamService) GetMemberByUserID(ctx context.Context, userID int64) (*TeamMember, error) {
	return s.memberRepo.GetByUserID(ctx, userID)
}

// UpdateMemberTags sets tags on a team member.
func (s *TeamService) UpdateMemberTags(ctx context.Context, teamID, memberID, operatorID int64, tags []string) (*TeamMember, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	member, err := s.memberRepo.GetByID(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if member.TeamID != teamID {
		return nil, ErrTeamMemberNotFound
	}
	if err := s.memberRepo.UpdateTags(ctx, member.ID, tags); err != nil {
		return nil, fmt.Errorf("update member tags: %w", err)
	}
	member.Tags = tags
	return member, nil
}

// SetMemberStatus flips a team member's user.status (team_admin path).
func (s *TeamService) SetMemberStatus(ctx context.Context, teamID, memberID, operatorID int64, status string) error {
	if status != StatusActive && status != StatusDisabled {
		return fmt.Errorf("invalid status %q", status)
	}
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return err
	}
	member, err := s.memberRepo.GetByID(ctx, memberID)
	if err != nil {
		return err
	}
	if member.TeamID != teamID {
		return ErrTeamMemberNotFound
	}
	user, err := s.userRepo.GetByID(ctx, member.UserID)
	if err != nil {
		return err
	}
	user.Status = status
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	s.invalidateAuthCacheForUser(ctx, member.UserID)
	return nil
}

// ResetMemberPassword resets a member's password (team_admin path).
func (s *TeamService) ResetMemberPassword(ctx context.Context, teamID, memberID, operatorID int64, newPassword string) error {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return err
	}
	member, err := s.memberRepo.GetByID(ctx, memberID)
	if err != nil {
		return err
	}
	if member.TeamID != teamID {
		return ErrTeamMemberNotFound
	}
	user, err := s.userRepo.GetByID(ctx, member.UserID)
	if err != nil {
		return err
	}
	if err := user.SetPassword(newPassword); err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.userRepo.Update(ctx, user)
}

// teamPurchaseBatchSize bounds each commit so a single tx doesn't lock the
// team/users/subscriptions rows for too long when buying for many members.
const teamPurchaseBatchSize = 50

// BulkPurchaseSubscriptionResult summarises a batched purchase/renew run so
// the caller can show progress (and partial outcome when balance ran out
// mid-batch).
type BulkPurchaseSubscriptionResult struct {
	Total         int                `json:"total"`          // # of target members at execution time
	Succeeded     int                `json:"succeeded"`      // # of subscriptions actually assigned/extended
	Subscriptions []UserSubscription `json:"subscriptions"`  // assigned/extended subscriptions in input order
	StoppedReason string             `json:"stopped_reason,omitempty"` // "" | "insufficient_balance"
}

// PurchaseSubscriptionsForMembers assigns or extends a plan for either a
// supplied set of members (requestedUserIDs) or every active member of the
// team (forAll=true). Work is committed in chunks of teamPurchaseBatchSize so
// the per-tx lock window stays short even for large teams.
//
// Renewal semantics: if a member already holds a subscription for
// plan.group_id, the term is extended (un-expired: accumulate; expired:
// restart from now). This also closes the double-charge gap on the old
// single-user path (idempotent assign that still deducted balance).
//
// Atomicity: each chunk is one tx. A mid-run "insufficient balance" stops the
// run gracefully — chunks already committed remain (the balance for those was
// spent), the result records Succeeded < Total + StoppedReason. Any other
// error rolls back the current chunk and surfaces to the caller; chunks
// already committed remain.
func (s *TeamService) PurchaseSubscriptionsForMembers(
	ctx context.Context,
	teamID int64,
	requestedUserIDs []int64,
	forAll bool,
	planID, operatorID int64,
) (*BulkPurchaseSubscriptionResult, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	if !forAll && len(requestedUserIDs) == 0 {
		return nil, fmt.Errorf("target user ids required when all_members=false")
	}
	if forAll && len(requestedUserIDs) > 0 {
		return nil, fmt.Errorf("cannot pass user_ids together with all_members=true")
	}

	plan, err := s.entClient.SubscriptionPlan.Get(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("get subscription plan: %w", err)
	}

	// Resolve the target list once. For forAll this is a "snapshot at start":
	// members added/removed during the run aren't picked up — keeps the
	// operator's intent ("buy for everyone right now") well-defined.
	var targetUserIDs []int64
	if forAll {
		targetUserIDs, err = s.memberRepo.ListMemberUserIDs(ctx, teamID, nil)
		if err != nil {
			return nil, fmt.Errorf("list team members: %w", err)
		}
	} else {
		seen := make(map[int64]struct{}, len(requestedUserIDs))
		deduped := make([]int64, 0, len(requestedUserIDs))
		for _, uid := range requestedUserIDs {
			if _, dup := seen[uid]; dup {
				continue
			}
			seen[uid] = struct{}{}
			deduped = append(deduped, uid)
		}
		matched, err := s.memberRepo.ListMemberUserIDs(ctx, teamID, deduped)
		if err != nil {
			return nil, fmt.Errorf("validate members: %w", err)
		}
		if len(matched) != len(deduped) {
			return nil, ErrTeamNotMember
		}
		targetUserIDs = deduped
	}

	result := &BulkPurchaseSubscriptionResult{
		Total:         len(targetUserIDs),
		Subscriptions: make([]UserSubscription, 0, len(targetUserIDs)),
	}
	if len(targetUserIDs) == 0 {
		return result, nil
	}

	// Pre-flight balance check — reject the obvious "not enough money" case
	// before any side effect. Concurrent deductions during the run can still
	// produce a mid-batch shortfall; that case is handled by the per-batch
	// overdraft check inside processPurchaseBatch.
	totalCost := plan.Price * float64(len(targetUserIDs))
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("read team balance: %w", err)
	}
	if team.Balance < totalCost {
		return nil, ErrTeamInsufficientBalance
	}

	for start := 0; start < len(targetUserIDs); start += teamPurchaseBatchSize {
		end := start + teamPurchaseBatchSize
		if end > len(targetUserIDs) {
			end = len(targetUserIDs)
		}
		chunk := targetUserIDs[start:end]

		subs, stopped, err := s.processPurchaseBatch(ctx, teamID, plan, planID, operatorID, chunk)
		if err != nil {
			return nil, err
		}
		result.Subscriptions = append(result.Subscriptions, subs...)
		result.Succeeded += len(subs)
		if stopped {
			result.StoppedReason = "insufficient_balance"
			break
		}
	}

	for i := 0; i < result.Succeeded; i++ {
		s.invalidateAuthCacheForUser(ctx, targetUserIDs[i])
	}
	return result, nil
}

// processPurchaseBatch commits one chunk in a single tx. Returns the chunk's
// subscriptions on success, (nil, true, nil) when the team balance went
// negative (rolled back, caller stops the run), or (nil, false, err) on a
// real error.
func (s *TeamService) processPurchaseBatch(
	ctx context.Context,
	teamID int64,
	plan *dbent.SubscriptionPlan,
	planID, operatorID int64,
	userIDs []int64,
) ([]UserSubscription, bool, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	chunkCost := plan.Price * float64(len(userIDs))
	updatedTeam, err := tx.Team.UpdateOneID(teamID).AddBalance(-chunkCost).Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("deduct team balance: %w", err)
	}
	if updatedTeam.Balance < 0 {
		return nil, true, nil
	}

	subs := make([]UserSubscription, 0, len(userIDs))
	for _, uid := range userIDs {
		sub, isRenewal, err := s.subService.AssignOrExtendSubscription(txCtx, &AssignSubscriptionInput{
			UserID:       uid,
			GroupID:      plan.GroupID,
			ValidityDays: plan.ValidityDays,
			AssignedBy:   operatorID,
			Notes:        fmt.Sprintf("team_purchase:team_id=%d:plan_id=%d", teamID, planID),
		})
		if err != nil {
			return nil, false, fmt.Errorf("assign subscription for user %d: %w", uid, err)
		}

		if _, err := tx.UserSubscription.UpdateOneID(sub.ID).SetTeamID(teamID).Save(ctx); err != nil {
			return nil, false, fmt.Errorf("stamp team_id on subscription: %w", err)
		}

		action := "purchase"
		if isRenewal {
			action = "renew"
		}
		note := fmt.Sprintf("%s plan %d for user %d", action, planID, uid)
		if _, err := tx.TeamBalanceLog.Create().
			SetTeamID(teamID).
			SetType(TeamBalanceLogTypeSubscriptionPurchase).
			SetAmount(-plan.Price).
			SetOperatorID(operatorID).
			SetTargetUserID(uid).
			SetNote(note).
			Save(ctx); err != nil {
			return nil, false, fmt.Errorf("create balance log: %w", err)
		}

		subs = append(subs, *sub)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return subs, false, nil
}

// ListTeamSubscriptions returns paginated subscriptions purchased by this team (team_admin path).
func (s *TeamService) ListTeamSubscriptions(ctx context.Context, teamID, operatorID int64, params pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, nil, err
	}
	subs, result, err := s.userSubRepo.ListByTeamID(ctx, teamID, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list team subscriptions: %w", err)
	}
	return subs, result, nil
}

// ListSubscriptionPlans returns all for-sale plans.
func (s *TeamService) ListSubscriptionPlans(ctx context.Context, teamID, operatorID int64) ([]*dbent.SubscriptionPlan, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	if s.paymentConfigService == nil {
		return nil, fmt.Errorf("payment config service not configured")
	}
	return s.paymentConfigService.ListPlansForSale(ctx)
}

// UpdateAvailableTags sets the team's tag set (team_admin path).
func (s *TeamService) UpdateAvailableTags(ctx context.Context, teamID, operatorID int64, tags []string) (*Team, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	if err := s.teamRepo.UpdateAvailableTags(ctx, teamID, tags); err != nil {
		return nil, fmt.Errorf("update available tags: %w", err)
	}
	return s.teamRepo.GetByID(ctx, teamID)
}

// ListMemberAPIKeys returns API keys owned by a member, scoped to the team.
func (s *TeamService) ListMemberAPIKeys(ctx context.Context, teamID, memberID, operatorID int64) ([]APIKey, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	member, err := s.memberRepo.GetByID(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if member.TeamID != teamID {
		return nil, ErrTeamMemberNotFound
	}
	keys, _, err := s.apiKeyRepo.ListByUserID(
		ctx,
		member.UserID,
		pagination.PaginationParams{Page: 1, PageSize: 500},
		APIKeyListFilters{TeamID: &teamID},
	)
	return keys, err
}

// ListMemberSubscriptions returns subscriptions held by a member.
func (s *TeamService) ListMemberSubscriptions(ctx context.Context, teamID, memberID, operatorID int64) ([]UserSubscription, error) {
	if err := s.requireTeamAdmin(ctx, teamID, operatorID); err != nil {
		return nil, err
	}
	member, err := s.memberRepo.GetByID(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if member.TeamID != teamID {
		return nil, ErrTeamMemberNotFound
	}
	return s.userSubRepo.ListByUserID(ctx, member.UserID)
}

// requireTeamAdmin enforces the team_admin permission for an operation.
// userID == 0 is treated as a trusted-caller bypass — used when the caller
// is sys admin (their permission has already been verified at the route
// middleware layer) or an internal background job. All public service
// methods that take operatorID forward it here, so always pass the real
// authenticated user id when calling on behalf of a team_admin.
func (s *TeamService) requireTeamAdmin(ctx context.Context, teamID, userID int64) error {
	if IsSysAdminCaller(ctx) {
		return nil
	}
	if userID == 0 {
		return ErrTeamAdminRequired
	}
	ok, err := s.adminRepo.ExistsByTeamAndUser(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamAdminRequired
	}
	return nil
}
