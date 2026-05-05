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

// MysekaiBirthdaySubscription stores the active birthday material monitor for
// one PJSK account. The unique region+uid constraint intentionally makes a new
// monitor overwrite the previous one.
type MysekaiBirthdaySubscription struct {
	ent.Schema
}

func (MysekaiBirthdaySubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mysekai_birthday_subscriptions"},
	}
}

func (MysekaiBirthdaySubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("region").MaxLen(7),
		field.String("uid").MaxLen(30),
		field.String("platform").MaxLen(32),
		field.String("platform_user_id").MaxLen(128),
		field.String("platform_group_id").MaxLen(128),
		field.String("cloud_bot_id").MaxLen(64),
		field.String("self_id").MaxLen(64),
		field.JSON("materials", []string{}),
		field.String("token").MaxLen(128),
		field.Bool("active").Default(true),
		field.Time("expires_at"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("cancelled_at").Optional().Nillable(),
	}
}

func (MysekaiBirthdaySubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("events", MysekaiBirthdaySubscriptionEvent.Type),
	}
}

func (MysekaiBirthdaySubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("region", "uid").Unique(),
		index.Fields("platform", "platform_user_id"),
		index.Fields("platform_group_id"),
		index.Fields("cloud_bot_id"),
		index.Fields("self_id"),
		index.Fields("expires_at"),
		index.Fields("active"),
	}
}
