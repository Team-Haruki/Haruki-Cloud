package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Outsidecharacter holds the schema definition for the Outsidecharacter entity.
type Outsidecharacter struct {
	ent.Schema
}

// Fields of the Outsidecharacter.
func (Outsidecharacter) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.String("name").Optional(),
	}
}

// Edges of the Outsidecharacter.
func (Outsidecharacter) Edges() []ent.Edge {
	return nil
}

// Indexes of the Outsidecharacter.
func (Outsidecharacter) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
