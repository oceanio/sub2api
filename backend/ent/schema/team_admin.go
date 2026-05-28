package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamAdmin holds the schema definition for team admin role assignments.
// v2: independent of TeamMember. A user can be admin of many teams.
type TeamAdmin struct {
	ent.Schema
}

func (TeamAdmin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_admins"},
	}
}

func (TeamAdmin) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (TeamAdmin) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("team_id"),
		field.Int64("user_id"),
	}
}

func (TeamAdmin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).
			Ref("admins").
			Field("team_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("team_admin_roles").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (TeamAdmin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id"),
		index.Fields("user_id"),
		// Uniqueness enforced via partial index in migration (WHERE deleted_at IS NULL).
		index.Fields("team_id", "user_id"),
	}
}
