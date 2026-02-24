package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventitem holds the schema definition for the Eventitem entity.
type Eventitem struct {
	ent.Schema
}

// Fields of the Eventitem.
func (Eventitem) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.String("name").Optional(),
		field.String("flavor_text").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("game_character_id").Optional(),
	}
}

// Edges of the Eventitem.
func (Eventitem) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventitem.
func (Eventitem) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
