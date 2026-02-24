package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Masterlesson holds the schema definition for the Masterlesson entity.
type Masterlesson struct {
	ent.Schema
}

// Fields of the Masterlesson.
func (Masterlesson) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.String("card_rarity_type").Optional(),
		field.Int64("master_rank").Optional(),
		field.Int64("power1_bonus_fixed").Optional(),
		field.Int64("power2_bonus_fixed").Optional(),
		field.Int64("power3_bonus_fixed").Optional(),
		field.Int64("character_rank_exp").Optional(),
		field.JSON("costs", []any{}).Optional(),
		field.JSON("rewards", []any{}).Optional(),
	}
}

// Edges of the Masterlesson.
func (Masterlesson) Edges() []ent.Edge {
	return nil
}

// Indexes of the Masterlesson.
func (Masterlesson) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("card_rarity_type", "master_rank").Fields("server_region"),
	}
}
