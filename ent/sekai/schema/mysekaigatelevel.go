package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaigatelevel holds the schema definition for the Mysekaigatelevel entity.
type Mysekaigatelevel struct {
	ent.Schema
}

// Fields of the Mysekaigatelevel.
func (Mysekaigatelevel) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("mysekai_gate_id").Optional(),
		field.Int64("level").Optional(),
		field.Int64("mysekai_gate_material_group_id").Optional(),
		field.Int64("mysekai_gate_character_visit_count_rate_id").Optional(),
		field.Float("power_bonus_rate").Optional(),
	}
}

// Edges of the Mysekaigatelevel.
func (Mysekaigatelevel) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaigatelevel.
func (Mysekaigatelevel) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
