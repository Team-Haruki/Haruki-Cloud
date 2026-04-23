package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ImageModCache struct {
	ent.Schema
}

func (ImageModCache) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "image_mod_cache"},
	}
}

func (ImageModCache) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Unique().Immutable(),
		field.String("url").MaxLen(2048).NotEmpty().Comment("Image URL that was moderated"),
		field.Int("haruki_user_id").Optional().Nillable().Comment("Haruki user who submitted the image"),
		field.String("result").MaxLen(20).NotEmpty().Comment("IMS suggestion: Pass/Review/Block"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (ImageModCache) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("url").Unique(),
		index.Fields("haruki_user_id"),
	}
}
