package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/dialect/entsql"
)

// Team holds the schema definition for the Team entity.
type Team struct {
	ent.Schema
}

func (Team) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "teams"},
	}
}

func (Team) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.Float("balance").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("Team balance in USD"),
		field.JSON("available_tags", []string{}).
			Optional().
			Comment("Team-defined tag set used for member tagging"),
		field.Int("max_members").
			Default(0).
			Comment("Cap on active paying members (0 = unlimited). Only sys admin can modify."),
	}
}

func (Team) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("members", TeamMember.Type),
		edge.To("admins", TeamAdmin.Type),
		edge.To("balance_logs", TeamBalanceLog.Type),
		edge.To("api_keys", APIKey.Type),
		edge.To("subscriptions", UserSubscription.Type),
	}
}

func (Team) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
	}
}
