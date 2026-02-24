package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Gacha holds the schema definition for the Gacha entity.
type Gacha struct {
	ent.Schema
}

// Fields of the Gacha.
func (Gacha) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("gacha_type").Optional(),
		field.String("name").Optional(),
		field.Int64("seq").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("gacha_card_rarity_rate_group_id").Optional(),
		field.Int64("start_at").Optional(),
		field.Int64("end_at").Optional(),
		field.Bool("is_show_period").Optional(),
		field.Int64("gacha_ceil_item_id").Optional(),
		field.Int64("wish_select_count").Optional(),
		field.Int64("wish_fixed_select_count").Optional(),
		field.Int64("wish_limited_select_count").Optional(),
		field.JSON("gacha_card_rarity_rates", []any{}).Optional(),
		field.JSON("gacha_details", []any{}).Optional(),
		field.JSON("gacha_behaviors", []any{}).Optional(),
		field.JSON("gacha_pickups", []any{}).Optional(),
		field.JSON("gacha_pickup_costumes", []any{}).Optional(),
		field.JSON("gacha_information", map[string]any{}).Optional(),
		field.Int64("drawable_gacha_hour").Optional(),
		field.Int64("gacha_bonus_id").Optional(),
		field.Int64("spin_limit").Optional(),
		field.Int64("gacha_bonus_item_receivable_reward_group_id").Optional(),
		field.Int64("gacha_freebie_group_id").Optional(),
		field.Int64("daily_spin_limit").Optional(),
	}
}

// Edges of the Gacha.
func (Gacha) Edges() []ent.Edge {
	return nil
}

// Indexes of the Gacha.
func (Gacha) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
