package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"haruki-cloud/internal/pjsk/drawing"
)

type ProfileBackground struct {
	ent.Schema
}

func (ProfileBackground) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("server").MaxLen(2).Comment("PJSK server region"),
		field.String("user_id").MaxLen(30).Comment("PJSK game user ID"),
		field.JSON("bg", &drawing.ProfileBgSettings{}).Comment("Profile card background settings stored as JSONB"),
	}
}

func (ProfileBackground) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("server", "user_id").Unique(),
	}
}
