package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Areaitemlevel holds the schema definition for the Areaitemlevel entity.
type Areaitemlevel struct {
	ent.Schema
}

// Fields of the Areaitemlevel.
func (Areaitemlevel) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("area_item_id").Optional(),
		field.Int64("level").Optional(),
		field.String("target_unit").Optional(),
		field.String("target_card_attr").Optional(),
		field.Int64("target_game_character_id").Optional(),
		field.Float("power1_bonus_rate").Optional(),
		field.Float("power1_all_match_bonus_rate").Optional(),
		field.Float("power2_bonus_rate").Optional(),
		field.Float("power2_all_match_bonus_rate").Optional(),
		field.Float("power3_bonus_rate").Optional(),
		field.Float("power3_all_match_bonus_rate").Optional(),
		field.String("sentence").Optional(),
	}
}

// Edges of the Areaitemlevel.
func (Areaitemlevel) Edges() []ent.Edge {
	return nil
}

// Indexes of the Areaitemlevel.
func (Areaitemlevel) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("area_item_id", "level").Fields("server_region"),
	}
}
