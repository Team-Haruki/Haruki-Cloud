package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaicharactertalk holds the schema definition for the Mysekaicharactertalk entity.
type Mysekaicharactertalk struct {
	ent.Schema
}

// Fields of the Mysekaicharactertalk.
func (Mysekaicharactertalk) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("mysekai_game_character_unit_group_id").Optional(),
		field.Int64("mysekai_character_talk_condition_group_id").Optional(),
		field.Int64("mysekai_site_group_id").Optional(),
		field.Int64("mysekai_character_talk_term_id").Optional(),
		field.Int64("character_archive_mysekai_character_talk_group_id").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("lua").Optional(),
		field.Bool("is_enabled_for_multi").Optional(),
	}
}

// Edges of the Mysekaicharactertalk.
func (Mysekaicharactertalk) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaicharactertalk.
func (Mysekaicharactertalk) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
