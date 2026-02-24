package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaiblueprint holds the schema definition for the Mysekaiblueprint entity.
type Mysekaiblueprint struct {
	ent.Schema
}

// Fields of the Mysekaiblueprint.
func (Mysekaiblueprint) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.JSON("mysekai_craft_type", map[string]any{}).Optional(),
		field.Int64("craft_target_id").Optional(),
		field.Bool("is_enable_sketch").Optional(),
		field.Bool("is_obtained_by_convert").Optional(),
		field.Int64("craft_count_limit").Optional(),
		field.Bool("is_available_without_possession").Optional(),
	}
}

// Edges of the Mysekaiblueprint.
func (Mysekaiblueprint) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaiblueprint.
func (Mysekaiblueprint) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
