package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MysekaiBirthdaySubscriptionEvent stores a filtered birthday-party upload
// event. HMES only forwards these event ids; Cloud remains the source of truth.
type MysekaiBirthdaySubscriptionEvent struct {
	ent.Schema
}

func (MysekaiBirthdaySubscriptionEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mysekai_birthday_subscription_events"},
	}
}

func (MysekaiBirthdaySubscriptionEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int("subscription_id"),
		field.String("region").MaxLen(7),
		field.String("uid").MaxLen(30),
		field.String("platform").MaxLen(32),
		field.String("platform_user_id").MaxLen(128),
		field.String("platform_group_id").MaxLen(128),
		field.String("cloud_bot_id").MaxLen(64),
		field.String("self_id").MaxLen(64),
		field.JSON("matched_material_ids", []int{}),
		field.Bool("empty_result").Default(false),
		field.Bytes("filtered_payload").Optional(),
		field.Time("upload_time"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("acknowledged_at").Optional().Nillable(),
	}
}

func (MysekaiBirthdaySubscriptionEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("subscription", MysekaiBirthdaySubscription.Type).
			Ref("events").
			Field("subscription_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (MysekaiBirthdaySubscriptionEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subscription_id"),
		index.Fields("region", "uid"),
		index.Fields("cloud_bot_id"),
		index.Fields("platform_group_id"),
		index.Fields("self_id"),
		index.Fields("created_at"),
		index.Fields("acknowledged_at"),
	}
}
