package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Level holds the schema definition for the Level entity.
type Level struct {
	ent.Schema
}

// Fields of the Level.
func (Level) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("level_type").Optional(),
		field.Int64("level").Optional(),
		field.Int64("total_exp").Optional(),
	}
}

// Edges of the Level.
func (Level) Edges() []ent.Edge {
	return nil
}

// Indexes of the Level.
func (Level) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("level_type", "level").Fields("server_region"),
	}
}
