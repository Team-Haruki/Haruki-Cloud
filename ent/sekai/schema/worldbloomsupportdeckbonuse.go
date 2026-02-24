package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Worldbloomsupportdeckbonuse holds the schema definition for the Worldbloomsupportdeckbonuse entity.
type Worldbloomsupportdeckbonuse struct {
	ent.Schema
}

// Fields of the Worldbloomsupportdeckbonuse.
func (Worldbloomsupportdeckbonuse) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.String("card_rarity_type").Optional(),
		field.JSON("world_bloom_support_deck_character_bonuses", []any{}).Optional(),
		field.JSON("world_bloom_support_deck_master_rank_bonuses", []any{}).Optional(),
		field.JSON("world_bloom_support_deck_skill_level_bonuses", []any{}).Optional(),
	}
}

// Edges of the Worldbloomsupportdeckbonuse.
func (Worldbloomsupportdeckbonuse) Edges() []ent.Edge {
	return nil
}

// Indexes of the Worldbloomsupportdeckbonuse.
func (Worldbloomsupportdeckbonuse) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("card_rarity_type").Fields("server_region"),
	}
}
