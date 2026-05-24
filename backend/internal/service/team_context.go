package service

import "context"

// Fork addition: separates the "trusted sys-admin caller" signal from the
// operator identity so audit logs (team_balance_logs.operator_id has a FK to
// users) can still record the real admin's user_id while requireTeamAdmin
// bypasses the team_admins membership check.
//
// Previously we overloaded operatorID==0 to mean both "bypass" and
// "anonymous"; writing 0 into an FK column then violated the constraint.

type sysAdminCallerCtxKey struct{}

// WithSysAdminCaller marks ctx as a trusted sys-admin caller. The HTTP layer
// sets this for /api/v1/admin/* paths after the admin-auth middleware has
// confirmed sys admin access. Service methods still receive the real
// authenticated user_id as operatorID for audit purposes.
func WithSysAdminCaller(ctx context.Context) context.Context {
	return context.WithValue(ctx, sysAdminCallerCtxKey{}, true)
}

// IsSysAdminCaller reports whether the context was produced via
// WithSysAdminCaller — i.e. the request came through an admin-auth-gated
// route and the team_admins membership check should be skipped.
func IsSysAdminCaller(ctx context.Context) bool {
	v, _ := ctx.Value(sysAdminCallerCtxKey{}).(bool)
	return v
}
