package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Musicdifficultie holds the schema definition for the Musicdifficultie entity.
type Musicdifficultie struct {
	ent.Schema
}

// Fields of the Musicdifficultie.
func (Musicdifficultie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("music_id").Optional(),
		field.String("music_difficulty").Optional(),
		field.Int64("play_level").Optional(),
		field.Int64("total_note_count").Optional(),
		field.Int64("release_condition_id").Optional(),
	}
}

// Edges of the Musicdifficultie.
func (Musicdifficultie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Musicdifficultie.
func (Musicdifficultie) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
