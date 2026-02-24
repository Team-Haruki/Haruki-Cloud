package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventdeckbonuse holds the schema definition for the Eventdeckbonuse entity.
type Eventdeckbonuse struct {
	ent.Schema
}

// Fields of the Eventdeckbonuse.
func (Eventdeckbonuse) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.Int64("game_character_unit_id").Optional(),
		field.String("card_attr").Optional(),
		field.Float("bonus_rate").Optional(),
	}
}

// Edges of the Eventdeckbonuse.
func (Eventdeckbonuse) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventdeckbonuse.
func (Eventdeckbonuse) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
