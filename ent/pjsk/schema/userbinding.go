package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserBinding struct {
	ent.Schema
}

func (UserBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int("haruki_user_id").Comment("Reference to users table"),
		field.Int("game_account_id").Optional().Nillable().Comment("Reference to game_accounts table"),
		field.Int("display_order").Default(0).Comment("Persistent binding display order"),
		field.Bool("visible").Default(true),
		field.Bool("suite_visible").Default(true).Comment("Controls visibility of suite/capture data"),
		field.Bool("mysekai_visible").Default(true).Comment("Controls visibility of mysekai private data"),
		field.Bool("verified").Default(false).Comment("Whether the game account has been verified"),
	}
}

func (UserBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("game_account", GameAccount.Type).
			Ref("bindings").
			Field("game_account_id").
			Unique(),
		edge.To("default_refs", UserDefaultBinding.Type),
	}
}

func (UserBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("haruki_user_id", "game_account_id").Unique(),
	}
}
