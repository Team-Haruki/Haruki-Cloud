package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Worldbloomsupportdeckuniteventlimitedbonuse holds the schema definition for the Worldbloomsupportdeckuniteventlimitedbonuse entity.
type Worldbloomsupportdeckuniteventlimitedbonuse struct {
	ent.Schema
}

// Fields of the Worldbloomsupportdeckuniteventlimitedbonuse.
func (Worldbloomsupportdeckuniteventlimitedbonuse) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.Int64("game_character_id").Optional(),
		field.Int64("card_id").Optional(),
		field.Float("bonus_rate").Optional(),
	}
}

// Edges of the Worldbloomsupportdeckuniteventlimitedbonuse.
func (Worldbloomsupportdeckuniteventlimitedbonuse) Edges() []ent.Edge {
	return nil
}

// Indexes of the Worldbloomsupportdeckuniteventlimitedbonuse.
func (Worldbloomsupportdeckuniteventlimitedbonuse) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
