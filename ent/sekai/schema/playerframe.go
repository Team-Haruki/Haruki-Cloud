package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Playerframe holds the schema definition for the Playerframe entity.
type Playerframe struct {
	ent.Schema
}

// Fields of the Playerframe.
func (Playerframe) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("player_frame_group_id").Optional(),
		field.String("description").Optional(),
		field.Int64("game_character_id").Optional(),
	}
}

// Edges of the Playerframe.
func (Playerframe) Edges() []ent.Edge {
	return nil
}

// Indexes of the Playerframe.
func (Playerframe) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
