package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventexchangesummarie holds the schema definition for the Eventexchangesummarie entity.
type Eventexchangesummarie struct {
	ent.Schema
}

// Fields of the Eventexchangesummarie.
func (Eventexchangesummarie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("start_at").Optional(),
		field.Int64("end_at").Optional(),
		field.JSON("event_exchanges", []any{}).Optional(),
	}
}

// Edges of the Eventexchangesummarie.
func (Eventexchangesummarie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventexchangesummarie.
func (Eventexchangesummarie) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
