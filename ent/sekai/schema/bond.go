package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Bond holds the schema definition for the Bond entity.
type Bond struct {
	ent.Schema
}

// Fields of the Bond.
func (Bond) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("group_id").Optional(),
		field.Int64("character_id1").Optional(),
		field.Int64("character_id2").Optional(),
	}
}

// Edges of the Bond.
func (Bond) Edges() []ent.Edge {
	return nil
}

// Indexes of the Bond.
func (Bond) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
