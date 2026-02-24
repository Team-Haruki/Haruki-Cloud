package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaigate holds the schema definition for the Mysekaigate entity.
type Mysekaigate struct {
	ent.Schema
}

// Fields of the Mysekaigate.
func (Mysekaigate) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("unit").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Mysekaigate.
func (Mysekaigate) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaigate.
func (Mysekaigate) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
