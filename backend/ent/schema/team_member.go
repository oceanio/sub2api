package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamMember holds the schema definition for the TeamMember entity.
// v2: represents PAYING membership only. Admin role lives in TeamAdmin.
type TeamMember struct {
	ent.Schema
}

func (TeamMember) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_members"},
	}
}

func (TeamMember) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (TeamMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("team_id"),
		field.Int64("user_id"),
		field.Float("sub_quota").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("Per-member spend cap in USD (0 = unlimited). Rate limiter only, does not pre-deduct team balance."),
		field.Float("sub_quota_used").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("Accumulated spend by this member against team balance"),
		field.JSON("tags", []string{}).
			Optional().
			Comment("Team-admin-managed labels, e.g. [\"engineering\", \"product\"]"),
	}
}

func (TeamMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).
			Ref("members").
			Field("team_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("team_memberships").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (TeamMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id"),
		index.Fields("user_id"),
		// Uniqueness enforced via partial index in migration (WHERE deleted_at IS NULL)
		index.Fields("team_id", "user_id"),
	}
}
