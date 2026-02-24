package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaigatecharacterlotterie holds the schema definition for the Mysekaigatecharacterlotterie entity.
type Mysekaigatecharacterlotterie struct {
	ent.Schema
}

// Fields of the Mysekaigatecharacterlotterie.
func (Mysekaigatecharacterlotterie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("mysekai_gate_id").Optional(),
		field.Int64("game_character_unit_id").Optional(),
		field.Int64("visitable_mysekai_gate_level").Optional(),
	}
}

// Edges of the Mysekaigatecharacterlotterie.
func (Mysekaigatecharacterlotterie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaigatecharacterlotterie.
func (Mysekaigatecharacterlotterie) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
