package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	entapikey "github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/team"
	"github.com/Wei-Shaw/sub2api/ent/teamadmin"
	"github.com/Wei-Shaw/sub2api/ent/teambalancelog"
	"github.com/Wei-Shaw/sub2api/ent/teammember"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/lib/pq"
)

// ── Team repository ────────────────────────────────────────────────────────────

type teamRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewTeamRepository(client *dbent.Client, db *sql.DB) service.TeamRepository {
	return &teamRepository{client: client, db: db}
}

func (r *teamRepository) Create(ctx context.Context, t *service.Team) error {
	created, err := r.client.Team.Create().
		SetName(t.Name).
		SetBalance(t.Balance).
		Save(ctx)
	if err != nil {
		return err
	}
	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	t.UpdatedAt = created.UpdatedAt
	return nil
}

// CreateWithInitialState atomically writes team + initial team_admin + (optionally)
// a paying-member record and/or a recharge balance log in a single ent transaction.
func (r *teamRepository) CreateWithInitialState(ctx context.Context, t *service.Team, initialAdminUserID int64, initialBalance float64, maxMembers int, alsoAddAsMember bool, operatorID int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	created, err := tx.Team.Create().
		SetName(t.Name).
		SetBalance(initialBalance).
		SetMaxMembers(maxMembers).
		Save(ctx)
	if err != nil {
		return err
	}

	if _, err := tx.TeamAdmin.Create().
		SetTeamID(created.ID).
		SetUserID(initialAdminUserID).
		Save(ctx); err != nil {
		return translatePersistenceError(err, nil, service.ErrTeamUserAlreadyAdmin)
	}

	if alsoAddAsMember {
		if _, err := tx.TeamMember.Create().
			SetTeamID(created.ID).
			SetUserID(initialAdminUserID).
			SetSubQuota(0).
			SetSubQuotaUsed(0).
			Save(ctx); err != nil {
			return translatePersistenceError(err, nil, service.ErrTeamUserAlreadyMember)
		}
	}

	if initialBalance > 0 {
		// Note is intentionally left NULL — the team-creation tx itself implies
		// "initial balance", so adding the literal string just clutters the log
		// without adding information.
		if _, err := tx.TeamBalanceLog.Create().
			SetTeamID(created.ID).
			SetType(service.TeamBalanceLogTypeRecharge).
			SetAmount(initialBalance).
			SetOperatorID(operatorID).
			Save(ctx); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	t.ID = created.ID
	t.Balance = initialBalance
	t.MaxMembers = maxMembers
	t.CreatedAt = created.CreatedAt
	t.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *teamRepository) GetByID(ctx context.Context, id int64) (*service.Team, error) {
	m, err := r.client.Team.Query().
		Where(team.IDEQ(id), team.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrTeamNotFound
		}
		return nil, err
	}
	t := teamEntityToService(m)
	// Populate member_count so overview pages get it without a second call.
	if c, err := r.CountActiveMembers(ctx, id); err == nil {
		t.MemberCount = c
	}
	return t, nil
}

// ListByIDs batch-loads teams in one query and enriches MemberCount in a single
// follow-up batched COUNT — avoids N+1 in the GetTeamsForAdmin / sidebar path.
func (r *teamRepository) ListByIDs(ctx context.Context, ids []int64) ([]service.Team, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.client.Team.Query().
		Where(team.IDIn(ids...), team.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.Team, 0, len(rows))
	keepIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, *teamEntityToService(row))
		keepIDs = append(keepIDs, row.ID)
	}
	if counts, err := r.CountActiveMembersByTeamIDs(ctx, keepIDs); err == nil {
		for i := range out {
			out[i].MemberCount = counts[out[i].ID]
		}
	}
	// Preserve the input order — callers may rely on it (e.g. menu ordering).
	if len(out) > 1 {
		pos := make(map[int64]int, len(ids))
		for i, id := range ids {
			pos[id] = i
		}
		sortByPos(out, pos)
	}
	return out, nil
}

// sortByPos rearranges teams to match the order encoded in pos. Teams not found
// in pos fall to the end (stable).
func sortByPos(items []service.Team, pos map[int64]int) {
	// simple insertion sort, fine for small N
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && pos[items[j-1].ID] > pos[items[j].ID]; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}

func (r *teamRepository) Update(ctx context.Context, t *service.Team) error {
	u := r.client.Team.UpdateOneID(t.ID).SetName(t.Name)
	// MaxMembers is part of the same admin-side update form; setting it to its
	// existing value is harmless and lets us avoid a second Update method.
	u = u.SetMaxMembers(t.MaxMembers)
	_, err := u.Save(ctx)
	return err
}

func (r *teamRepository) SoftDelete(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.Team.UpdateOneID(id).SetDeletedAt(now).Save(ctx)
	return err
}

func (r *teamRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Team, *pagination.PaginationResult, error) {
	q := r.client.Team.Query().Where(team.DeletedAtIsNil())

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(team.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.Team, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, *teamEntityToService(row))
		ids = append(ids, row.ID)
	}
	if counts, err := r.CountActiveMembersByTeamIDs(ctx, ids); err == nil {
		for i := range out {
			out[i].MemberCount = counts[out[i].ID]
		}
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *teamRepository) AddBalance(ctx context.Context, id int64, amount float64) (float64, error) {
	var newBalance float64
	err := r.db.QueryRowContext(ctx, `
		UPDATE teams
		SET balance = balance + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance + $1 >= 0
		RETURNING balance
	`, amount, id).Scan(&newBalance)
	if err == sql.ErrNoRows {
		return 0, service.ErrTeamInsufficientBalance
	}
	return newBalance, err
}

func (r *teamRepository) UpdateAvailableTags(ctx context.Context, id int64, tags []string) error {
	u := r.client.Team.UpdateOneID(id)
	if len(tags) > 0 {
		u.SetAvailableTags(tags)
	} else {
		u.ClearAvailableTags()
	}
	_, err := u.Save(ctx)
	return err
}

func (r *teamRepository) CountActiveMembers(ctx context.Context, id int64) (int64, error) {
	count, err := r.client.TeamMember.Query().
		Where(teammember.TeamIDEQ(id), teammember.DeletedAtIsNil()).
		Count(ctx)
	return int64(count), err
}

func (r *teamRepository) CountActiveMembersByTeamIDs(ctx context.Context, ids []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT team_id, COUNT(*) FROM team_members
		WHERE team_id = ANY($1) AND deleted_at IS NULL
		GROUP BY team_id
	`, pq.Int64Array(ids))
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tid, c int64
		if err := rows.Scan(&tid, &c); err == nil {
			out[tid] = c
		}
	}
	return out, rows.Err()
}

func teamEntityToService(m *dbent.Team) *service.Team {
	if m == nil {
		return nil
	}
	return &service.Team{
		ID:            m.ID,
		Name:          m.Name,
		Balance:       m.Balance,
		AvailableTags: m.AvailableTags,
		MaxMembers:    m.MaxMembers,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// ── TeamMember repository ──────────────────────────────────────────────────────

type teamMemberRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewTeamMemberRepository(client *dbent.Client, db *sql.DB) service.TeamMemberRepository {
	return &teamMemberRepository{client: client, db: db}
}

func (r *teamMemberRepository) Create(ctx context.Context, m *service.TeamMember) error {
	client := clientFromContext(ctx, r.client)
	b := client.TeamMember.Create().
		SetTeamID(m.TeamID).
		SetUserID(m.UserID).
		SetSubQuota(m.SubQuota).
		SetSubQuotaUsed(0)
	if len(m.Tags) > 0 {
		b.SetTags(m.Tags)
	}
	created, err := b.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrTeamUserAlreadyMember)
	}
	m.ID = created.ID
	m.CreatedAt = created.CreatedAt
	m.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *teamMemberRepository) GetByID(ctx context.Context, id int64) (*service.TeamMember, error) {
	m, err := r.client.TeamMember.Query().
		Where(teammember.IDEQ(id), teammember.DeletedAtIsNil()).
		WithUser().
		WithTeam().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrTeamMemberNotFound
		}
		return nil, err
	}
	return teamMemberEntityToService(m), nil
}

func (r *teamMemberRepository) GetByUserID(ctx context.Context, userID int64) (*service.TeamMember, error) {
	m, err := r.client.TeamMember.Query().
		Where(teammember.UserIDEQ(userID), teammember.DeletedAtIsNil()).
		WithUser().
		WithTeam().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrTeamMemberNotFound
		}
		return nil, err
	}
	return teamMemberEntityToService(m), nil
}

func (r *teamMemberRepository) GetByTeamAndUserID(ctx context.Context, teamID, userID int64) (*service.TeamMember, error) {
	m, err := r.client.TeamMember.Query().
		Where(
			teammember.TeamIDEQ(teamID),
			teammember.UserIDEQ(userID),
			teammember.DeletedAtIsNil(),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrTeamMemberNotFound
		}
		return nil, err
	}
	return teamMemberEntityToService(m), nil
}

func (r *teamMemberRepository) ListMemberUserIDs(ctx context.Context, teamID int64, filter []int64) ([]int64, error) {
	q := r.client.TeamMember.Query().
		Where(teammember.TeamIDEQ(teamID), teammember.DeletedAtIsNil())
	if len(filter) > 0 {
		q = q.Where(teammember.UserIDIn(filter...))
	}
	var ids []int64
	if err := q.Select(teammember.FieldUserID).Scan(ctx, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *teamMemberRepository) UpdateTags(ctx context.Context, id int64, tags []string) error {
	u := r.client.TeamMember.UpdateOneID(id)
	if len(tags) > 0 {
		u.SetTags(tags)
	} else {
		u.ClearTags()
	}
	_, err := u.Save(ctx)
	return err
}

func (r *teamMemberRepository) SoftDelete(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.TeamMember.UpdateOneID(id).SetDeletedAt(now).Save(ctx)
	return err
}

func (r *teamMemberRepository) ListByTeamID(ctx context.Context, teamID int64, params pagination.PaginationParams) ([]service.TeamMember, *pagination.PaginationResult, error) {
	q := r.client.TeamMember.Query().
		Where(teammember.TeamIDEQ(teamID), teammember.DeletedAtIsNil())

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.
		WithUser().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Asc(teammember.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.TeamMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, *teamMemberEntityToService(row))
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *teamMemberRepository) ListByTeamIDFiltered(ctx context.Context, teamID int64, tags []string, params pagination.PaginationParams) ([]service.TeamMember, *pagination.PaginationResult, error) {
	q := r.client.TeamMember.Query().
		Where(teammember.TeamIDEQ(teamID), teammember.DeletedAtIsNil())

	if len(tags) > 0 {
		// "??" escapes to a literal "?" so the JSONB operator "?|" survives ent's
		// placeholder rewriting; the trailing "?" then binds the StringArray.
		q = q.Where(func(sel *entsql.Selector) {
			sel.Where(entsql.ExprP(sel.C(teammember.FieldTags)+" ??| ?", pq.StringArray(tags)))
		})
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.
		WithUser().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Asc(teammember.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.TeamMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, *teamMemberEntityToService(row))
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *teamMemberRepository) UpdateSubQuota(ctx context.Context, id int64, subQuota float64) error {
	_, err := r.client.TeamMember.UpdateOneID(id).SetSubQuota(subQuota).Save(ctx)
	return err
}

func (r *teamMemberRepository) IncrementSubQuotaUsed(ctx context.Context, id int64, cost float64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE team_members SET sub_quota_used = sub_quota_used + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, cost, id)
	return err
}

func (r *teamMemberRepository) GetActivityStats(ctx context.Context, teamID int64, userIDs []int64) (map[int64]service.MemberActivityStats, error) {
	out := make(map[int64]service.MemberActivityStats, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, COUNT(*) FROM user_subscriptions
		WHERE user_id = ANY($1)
		  AND deleted_at IS NULL
		  AND status = 'active'
		  AND expires_at > NOW()
		GROUP BY user_id
	`, pq.Int64Array(userIDs))
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var uid int64
		var count int
		if err := rows.Scan(&uid, &count); err == nil {
			s := out[uid]
			s.ActiveSubscriptions = count
			out[uid] = s
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	rows2, err := r.db.QueryContext(ctx, `
		SELECT user_id, MAX(last_used_at) FROM api_keys
		WHERE user_id = ANY($1)
		  AND team_id = $2
		  AND deleted_at IS NULL
		GROUP BY user_id
	`, pq.Int64Array(userIDs), teamID)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows2.Close() }()
	for rows2.Next() {
		var uid int64
		var t sql.NullTime
		if err := rows2.Scan(&uid, &t); err == nil && t.Valid {
			s := out[uid]
			lastUsed := t.Time
			s.LastActiveAt = &lastUsed
			out[uid] = s
		}
	}
	return out, rows2.Err()
}

func (r *teamMemberRepository) DisableAPIKeysByTeamID(ctx context.Context, userID, teamID int64) error {
	_, err := r.client.APIKey.Update().
		Where(
			entapikey.UserIDEQ(userID),
			entapikey.TeamIDEQ(teamID),
			entapikey.DeletedAtIsNil(),
		).
		SetStatus("disabled").
		Save(ctx)
	return err
}

func teamMemberEntityToService(m *dbent.TeamMember) *service.TeamMember {
	if m == nil {
		return nil
	}
	out := &service.TeamMember{
		ID:           m.ID,
		TeamID:       m.TeamID,
		UserID:       m.UserID,
		SubQuota:     m.SubQuota,
		SubQuotaUsed: m.SubQuotaUsed,
		Tags:         m.Tags,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
	if m.Edges.User != nil {
		u := m.Edges.User
		out.User = &service.TeamMemberUser{
			ID:       u.ID,
			Email:    u.Email,
			Username: u.Username,
			Status:   u.Status,
		}
	}
	if m.Edges.Team != nil {
		out.Team = teamEntityToService(m.Edges.Team)
	}
	return out
}

// ── TeamAdmin repository ──────────────────────────────────────────────────────

type teamAdminRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewTeamAdminRepository(client *dbent.Client, db *sql.DB) service.TeamAdminRepository {
	return &teamAdminRepository{client: client, db: db}
}

func (r *teamAdminRepository) Create(ctx context.Context, teamID, userID int64) (*service.TeamAdmin, error) {
	client := clientFromContext(ctx, r.client)
	created, err := client.TeamAdmin.Create().
		SetTeamID(teamID).
		SetUserID(userID).
		Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, service.ErrTeamUserAlreadyAdmin)
	}
	return &service.TeamAdmin{
		ID:        created.ID,
		TeamID:    created.TeamID,
		UserID:    created.UserID,
		CreatedAt: created.CreatedAt,
	}, nil
}

func (r *teamAdminRepository) SoftDelete(ctx context.Context, teamID, userID int64) error {
	now := time.Now()
	n, err := r.client.TeamAdmin.Update().
		Where(
			teamadmin.TeamIDEQ(teamID),
			teamadmin.UserIDEQ(userID),
			teamadmin.DeletedAtIsNil(),
		).
		SetDeletedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrTeamAdminNotFound
	}
	return nil
}

func (r *teamAdminRepository) ExistsByTeamAndUser(ctx context.Context, teamID, userID int64) (bool, error) {
	return r.client.TeamAdmin.Query().
		Where(
			teamadmin.TeamIDEQ(teamID),
			teamadmin.UserIDEQ(userID),
			teamadmin.DeletedAtIsNil(),
		).
		Exist(ctx)
}

func (r *teamAdminRepository) ListByTeamID(ctx context.Context, teamID int64) ([]service.TeamAdmin, error) {
	rows, err := r.client.TeamAdmin.Query().
		Where(teamadmin.TeamIDEQ(teamID), teamadmin.DeletedAtIsNil()).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.TeamAdmin, 0, len(rows))
	for _, row := range rows {
		ta := service.TeamAdmin{
			ID:        row.ID,
			TeamID:    row.TeamID,
			UserID:    row.UserID,
			CreatedAt: row.CreatedAt,
		}
		if row.Edges.User != nil {
			u := row.Edges.User
			ta.User = &service.TeamMemberUser{
				ID:       u.ID,
				Email:    u.Email,
				Username: u.Username,
				Status:   u.Status,
			}
		}
		out = append(out, ta)
	}
	return out, nil
}

func (r *teamAdminRepository) ListTeamIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.client.TeamAdmin.Query().
		Where(teamadmin.UserIDEQ(userID), teamadmin.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TeamID)
	}
	return ids, nil
}

func (r *teamAdminRepository) CountActiveByTeamID(ctx context.Context, teamID int64) (int64, error) {
	count, err := r.client.TeamAdmin.Query().
		Where(teamadmin.TeamIDEQ(teamID), teamadmin.DeletedAtIsNil()).
		Count(ctx)
	return int64(count), err
}

func (r *teamAdminRepository) IsAdminMap(ctx context.Context, teamID int64, userIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id FROM team_admins
		WHERE team_id = $1 AND user_id = ANY($2) AND deleted_at IS NULL
	`, teamID, pq.Int64Array(userIDs))
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err == nil {
			out[uid] = true
		}
	}
	return out, rows.Err()
}

// ── TeamBalanceLog repository ──────────────────────────────────────────────────

type teamBalanceLogRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewTeamBalanceLogRepository(client *dbent.Client, db *sql.DB) service.TeamBalanceLogRepository {
	return &teamBalanceLogRepository{client: client, db: db}
}

func (r *teamBalanceLogRepository) Create(ctx context.Context, log *service.TeamBalanceLog) error {
	b := r.client.TeamBalanceLog.Create().
		SetTeamID(log.TeamID).
		SetType(log.Type).
		SetAmount(log.Amount).
		SetOperatorID(log.OperatorID).
		SetNillableTargetUserID(log.TargetUserID).
		SetNillableNote(log.Note)
	created, err := b.Save(ctx)
	if err != nil {
		return fmt.Errorf("create team balance log: %w", err)
	}
	log.ID = created.ID
	log.CreatedAt = created.CreatedAt
	return nil
}

func (r *teamBalanceLogRepository) ListByTeamID(ctx context.Context, teamID int64, params pagination.PaginationParams) ([]service.TeamBalanceLog, *pagination.PaginationResult, error) {
	q := r.client.TeamBalanceLog.Query().Where(teambalancelog.TeamIDEQ(teamID))

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.
		Order(dbent.Desc(teambalancelog.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]service.TeamBalanceLog, 0, len(rows))
	userIDSet := make(map[int64]struct{})
	for _, row := range rows {
		out = append(out, *teamBalanceLogEntityToService(row))
		userIDSet[row.OperatorID] = struct{}{}
		if row.TargetUserID != nil {
			userIDSet[*row.TargetUserID] = struct{}{}
		}
	}
	if len(userIDSet) > 0 {
		users, err := r.loadUsers(ctx, userIDSet)
		if err == nil {
			for i := range out {
				if u, ok := users[out[i].OperatorID]; ok {
					out[i].Operator = u
				}
				if out[i].TargetUserID != nil {
					if u, ok := users[*out[i].TargetUserID]; ok {
						out[i].TargetUser = u
					}
				}
			}
		}
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *teamBalanceLogRepository) loadUsers(ctx context.Context, set map[int64]struct{}) (map[int64]*service.TeamMemberUser, error) {
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, email, username FROM users WHERE id = ANY($1)`, pq.Int64Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]*service.TeamMemberUser, len(ids))
	for rows.Next() {
		var u service.TeamMemberUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Username); err == nil {
			out[u.ID] = &u
		}
	}
	return out, rows.Err()
}

func teamBalanceLogEntityToService(m *dbent.TeamBalanceLog) *service.TeamBalanceLog {
	if m == nil {
		return nil
	}
	return &service.TeamBalanceLog{
		ID:           m.ID,
		TeamID:       m.TeamID,
		Type:         m.Type,
		Amount:       m.Amount,
		OperatorID:   m.OperatorID,
		TargetUserID: m.TargetUserID,
		Note:         m.Note,
		CreatedAt:    m.CreatedAt,
	}
}
