package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProfileBGCleanup records object ownership before upload and durable deletion
// debt after the account reference is replaced. It intentionally has no account
// foreign key: deleting an account must not discard pending cleanup.
type ProfileBGCleanup struct{ ent.Schema }

func (ProfileBGCleanup) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("object_path").MaxLen(512).Unique(),
		field.Int("game_account_id"),
		field.Enum("state").Values("uploading", "pending", "deleting"),
		field.Time("not_before"),
		field.Int("attempts").Default(0),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (ProfileBGCleanup) Indexes() []ent.Index {
	return []ent.Index{index.Fields("not_before", "state")}
}
