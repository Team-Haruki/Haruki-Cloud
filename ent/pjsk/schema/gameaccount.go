package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"haruki-cloud/internal/pjsk/drawing"
)

type GameAccount struct {
	ent.Schema
}

func (GameAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("user_id").MaxLen(30).Comment("PJSK game user ID"),
		field.String("server").MaxLen(2).Comment("PJSK server region"),
		field.JSON("bg", &drawing.ProfileBgSettings{}).
			Optional().
			Comment("Profile card background settings stored as JSONB"),
	}
}

func (GameAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("bindings", UserBinding.Type),
	}
}

func (GameAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("server", "user_id").Unique(),
	}
}
