package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cardmysekaicanvasbonuse holds the schema definition for the Cardmysekaicanvasbonuse entity.
type Cardmysekaicanvasbonuse struct {
	ent.Schema
}

// Fields of the Cardmysekaicanvasbonuse.
func (Cardmysekaicanvasbonuse) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("card_rarity_type").Optional(),
		field.Int64("power1_bonus_fixed").Optional(),
		field.Int64("power2_bonus_fixed").Optional(),
		field.Int64("power3_bonus_fixed").Optional(),
	}
}

// Edges of the Cardmysekaicanvasbonuse.
func (Cardmysekaicanvasbonuse) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cardmysekaicanvasbonuse.
func (Cardmysekaicanvasbonuse) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
