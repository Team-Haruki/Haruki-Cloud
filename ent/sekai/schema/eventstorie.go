package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventstorie holds the schema definition for the Eventstorie entity.
type Eventstorie struct {
	ent.Schema
}

// Fields of the Eventstorie.
func (Eventstorie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.String("outline").Optional(),
		field.Int64("banner_game_character_unit_id").Optional(),
		field.String("assetbundle_name").Optional(),
		field.JSON("event_story_episodes", []any{}).Optional(),
	}
}

// Edges of the Eventstorie.
func (Eventstorie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventstorie.
func (Eventstorie) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
