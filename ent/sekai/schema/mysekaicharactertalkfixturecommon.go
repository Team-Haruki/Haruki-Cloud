package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaicharactertalkfixturecommon holds the schema definition for the Mysekaicharactertalkfixturecommon entity.
type Mysekaicharactertalkfixturecommon struct {
	ent.Schema
}

// Fields of the Mysekaicharactertalkfixturecommon.
func (Mysekaicharactertalkfixturecommon) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("game_character_unit_id").Optional(),
		field.JSON("mysekai_character_talk_fixture_common_type", map[string]any{}).Optional(),
		field.Int64("mysekai_character_talk_fixture_common_mysekai_fixture_group_id").Optional(),
		field.Int64("mysekai_character_talk_fixture_common_tweet_group_id").Optional(),
	}
}

// Edges of the Mysekaicharactertalkfixturecommon.
func (Mysekaicharactertalkfixturecommon) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaicharactertalkfixturecommon.
func (Mysekaicharactertalkfixturecommon) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
