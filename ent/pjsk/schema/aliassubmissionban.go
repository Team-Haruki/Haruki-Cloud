package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AliasSubmissionBan records IM users who may not submit new aliases.
type AliasSubmissionBan struct {
	ent.Schema
}

func (AliasSubmissionBan) Fields() []ent.Field {
	return []ent.Field{
		field.String("platform").MaxLen(20),
		field.String("platform_user_id").MaxLen(100),
		field.String("banned_by").MaxLen(100),
		field.Time("banned_at"),
	}
}

func (AliasSubmissionBan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "platform_user_id").Unique(),
	}
}

func (AliasSubmissionBan) Edges() []ent.Edge {
	return nil
}
