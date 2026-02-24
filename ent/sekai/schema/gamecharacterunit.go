package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Gamecharacterunit holds the schema definition for the Gamecharacterunit entity.
type Gamecharacterunit struct {
	ent.Schema
}

// Fields of the Gamecharacterunit.
func (Gamecharacterunit) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("game_character_id").Optional(),
		field.String("unit").Optional(),
		field.String("color_code").Optional(),
		field.String("skin_color_code").Optional(),
		field.String("skin_shadow_color_code1").Optional(),
		field.String("skin_shadow_color_code2").Optional(),
	}
}

// Edges of the Gamecharacterunit.
func (Gamecharacterunit) Edges() []ent.Edge {
	return nil
}

// Indexes of the Gamecharacterunit.
func (Gamecharacterunit) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
