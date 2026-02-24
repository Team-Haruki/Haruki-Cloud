package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Skill holds the schema definition for the Skill entity.
type Skill struct {
	ent.Schema
}

// Fields of the Skill.
func (Skill) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("short_description").Optional(),
		field.String("description").Optional(),
		field.String("description_sprite_name").Optional(),
		field.Int64("skill_filter_id").Optional(),
		field.JSON("skill_effects", []any{}).Optional(),
	}
}

// Edges of the Skill.
func (Skill) Edges() []ent.Edge {
	return nil
}

// Indexes of the Skill.
func (Skill) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
