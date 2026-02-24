package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Character2D holds the schema definition for the Character2D entity.
type Character2D struct {
	ent.Schema
}

// Fields of the Character2D.
func (Character2D) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("character_type").Optional(),
		field.Bool("is_next_grade").Optional(),
		field.Int64("character_id").Optional(),
		field.String("unit").Optional(),
		field.Bool("is_enabled_flip_display").Optional(),
		field.String("asset_name").Optional(),
		field.String("character_icon_assetbundle_name").Optional(),
	}
}

// Edges of the Character2D.
func (Character2D) Edges() []ent.Edge {
	return nil
}

// Indexes of the Character2D.
func (Character2D) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
