package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"haruki-cloud/utils/drawing"
)

type UserBinding struct {
	ent.Schema
}

func (UserBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int("haruki_user_id").Comment("Reference to users table"),
		field.String("user_id").MaxLen(30),
		field.String("server").MaxLen(2),
		field.Bool("visible").Default(true),
		field.Bool("suite_visible").Default(true).Comment("Controls visibility of suite/capture data"),
		field.JSON("bg", &drawing.ProfileBgSettings{}).Optional().Comment("Profile card background settings stored as JSONB"),
		field.Bool("verified").Default(false).Comment("Whether the game account has been verified"),
	}
}

func (UserBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("default_refs", UserDefaultBinding.Type),
	}
}

func (UserBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("haruki_user_id", "server", "user_id").Unique(),
	}
}
