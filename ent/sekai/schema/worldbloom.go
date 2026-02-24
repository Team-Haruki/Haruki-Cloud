package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Worldbloom holds the schema definition for the Worldbloom entity.
type Worldbloom struct {
	ent.Schema
}

// Fields of the Worldbloom.
func (Worldbloom) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.Int64("game_character_id").Optional(),
		field.String("world_bloom_chapter_type").Optional(),
		field.Int64("chapter_no").Optional(),
		field.Int64("chapter_start_at").Optional(),
		field.Int64("aggregate_at").Optional(),
		field.Int64("chapter_end_at").Optional(),
		field.Bool("is_supplemental").Optional(),
		field.Int64("costume2_d_id").Optional(),
	}
}

// Edges of the Worldbloom.
func (Worldbloom) Edges() []ent.Edge {
	return nil
}

// Indexes of the Worldbloom.
func (Worldbloom) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
