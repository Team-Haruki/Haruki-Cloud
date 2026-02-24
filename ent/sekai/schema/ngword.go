package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Ngword holds the schema definition for the Ngword entity.
type Ngword struct {
	ent.Schema
}

// Fields of the Ngword.
func (Ngword) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("word").Optional(),
	}
}

// Edges of the Ngword.
func (Ngword) Edges() []ent.Edge {
	return nil
}

// Indexes of the Ngword.
func (Ngword) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("word").Fields("server_region"),
	}
}
