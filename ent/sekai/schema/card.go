package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Card holds the schema definition for the Card entity.
type Card struct {
	ent.Schema
}

// Fields of the Card.
func (Card) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("character_id").Optional(),
		field.String("card_rarity_type").Optional(),
		field.Int64("special_training_power1_bonus_fixed").Optional(),
		field.Int64("special_training_power2_bonus_fixed").Optional(),
		field.Int64("special_training_power3_bonus_fixed").Optional(),
		field.String("attr").Optional(),
		field.String("support_unit").Optional(),
		field.Int64("skill_id").Optional(),
		field.String("card_skill_name").Optional(),
		field.String("prefix").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("gacha_phrase").Optional(),
		field.String("flavor_text").Optional(),
		field.Int64("release_at").Optional(),
		field.Int64("archive_published_at").Optional(),
		field.Int64("card_supply_id").Optional(),
		field.String("card_parameters").Optional(),
		field.JSON("special_training_costs", []any{}).Optional(),
		field.JSON("master_lesson_achieve_resources", []any{}).Optional(),
		field.String("initial_special_training_status").Optional(),
		field.String("archive_display_type").Optional(),
		field.Int64("special_training_skill_id").Optional(),
		field.String("special_training_skill_name").Optional(),
		field.Int64("special_training_reward_resource_box_id").Optional(),
	}
}

// Edges of the Card.
func (Card) Edges() []ent.Edge {
	return nil
}

// Indexes of the Card.
func (Card) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
