package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Gamecharacter holds the schema definition for the Gamecharacter entity.
type Gamecharacter struct {
	ent.Schema
}

// Fields of the Gamecharacter.
func (Gamecharacter) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("resource_id").Optional(),
		field.String("first_name").Optional(),
		field.String("given_name").Optional(),
		field.String("first_name_ruby").Optional(),
		field.String("given_name_ruby").Optional(),
		field.String("first_name_english").Optional(),
		field.String("given_name_english").Optional(),
		field.String("gender").Optional(),
		field.Float("height").Optional(),
		field.Float("live2_d_height_adjustment").Optional(),
		field.String("figure").Optional(),
		field.String("breast_size").Optional(),
		field.String("model_name").Optional(),
		field.String("unit").Optional(),
		field.String("support_unit_type").Optional(),
	}
}

// Edges of the Gamecharacter.
func (Gamecharacter) Edges() []ent.Edge {
	return nil
}

// Indexes of the Gamecharacter.
func (Gamecharacter) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
