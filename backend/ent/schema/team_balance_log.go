package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TeamBalanceLog holds the schema definition for the TeamBalanceLog entity.
// Append-only; no updates or soft deletes.
type TeamBalanceLog struct {
	ent.Schema
}

func (TeamBalanceLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_balance_logs"},
	}
}

func (TeamBalanceLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("team_id"),
		field.String("type").
			MaxLen(30).
			Comment("recharge | subscription_purchase"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("Positive = credit, negative = debit"),
		field.Int64("operator_id").
			Comment("User who triggered the operation"),
		field.Int64("target_user_id").
			Optional().
			Nillable().
			Comment("Recipient user for subscription_purchase"),
		field.String("note").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (TeamBalanceLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).
			Ref("balance_logs").
			Field("team_id").
			Unique().
			Required(),
	}
}

func (TeamBalanceLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id"),
		index.Fields("team_id", "created_at"),
		index.Fields("operator_id"),
	}
}
