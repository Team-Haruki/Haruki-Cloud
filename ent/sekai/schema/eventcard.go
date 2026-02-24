package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventcard holds the schema definition for the Eventcard entity.
type Eventcard struct {
	ent.Schema
}

// Fields of the Eventcard.
func (Eventcard) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("card_id").Optional(),
		field.Int64("event_id").Optional(),
		field.Float("bonus_rate").Optional(),
		field.Float("leader_bonus_rate").Optional(),
		field.Bool("is_display_card_story").Optional(),
	}
}

// Edges of the Eventcard.
func (Eventcard) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventcard.
func (Eventcard) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("event_id", "card_id").Fields("server_region"),
	}
}
