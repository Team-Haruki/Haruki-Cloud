package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixture holds the schema definition for the Mysekaifixture entity.
type Mysekaifixture struct {
	ent.Schema
}

// Fields of the Mysekaifixture.
func (Mysekaifixture) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.JSON("mysekai_fixture_type", map[string]any{}).Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
		field.String("flavor_text").Optional(),
		field.Int64("seq").Optional(),
		field.JSON("grid_size", map[string]any{}).Optional(),
		field.Int64("mysekai_fixture_main_genre_id").Optional(),
		field.Int64("mysekai_fixture_sub_genre_id").Optional(),
		field.JSON("mysekai_fixture_handle_type", map[string]any{}).Optional(),
		field.JSON("mysekai_settable_site_type", map[string]any{}).Optional(),
		field.JSON("mysekai_settable_layout_type", map[string]any{}).Optional(),
		field.JSON("mysekai_fixture_put_type", map[string]any{}).Optional(),
		field.JSON("mysekai_fixture_another_colors", []any{}).Optional(),
		field.Int64("mysekai_fixture_put_sound_id").Optional(),
		field.Int64("mysekai_fixture_footstep_id").Optional(),
		field.JSON("mysekai_fixture_tag_group", map[string]any{}).Optional(),
		field.Bool("is_assembled").Optional(),
		field.Bool("is_disassembled").Optional(),
		field.JSON("mysekai_fixture_player_action_type", map[string]any{}).Optional(),
		field.Bool("is_game_character_action").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("first_put_cost").Optional(),
		field.Int64("second_put_cost").Optional(),
		field.String("color_code").Optional(),
		field.Int64("mysekai_fixture_game_character_group_performance_bonus_id").Optional(),
	}
}

// Edges of the Mysekaifixture.
func (Mysekaifixture) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixture.
func (Mysekaifixture) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
