package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cardsupplie holds the schema definition for the Cardsupplie entity.
type Cardsupplie struct {
	ent.Schema
}

// Fields of the Cardsupplie.
func (Cardsupplie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("card_supply_type").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Cardsupplie.
func (Cardsupplie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cardsupplie.
func (Cardsupplie) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
