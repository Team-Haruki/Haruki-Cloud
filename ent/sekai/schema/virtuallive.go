package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Virtuallive holds the schema definition for the Virtuallive entity.
type Virtuallive struct {
	ent.Schema
}

// Fields of the Virtuallive.
func (Virtuallive) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("virtual_live_type").Optional(),
		field.String("virtual_live_platform").Optional(),
		field.Int64("seq").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("screen_mv_music_vocal_id").Optional(),
		field.Int64("start_at").Optional(),
		field.Int64("end_at").Optional(),
		field.Int64("ranking_announce_at").Optional(),
		field.JSON("virtual_live_setlists", []any{}).Optional(),
		field.JSON("virtual_live_beginner_schedules", []any{}).Optional(),
		field.JSON("virtual_live_schedules", []any{}).Optional(),
		field.JSON("virtual_live_characters", []any{}).Optional(),
		field.JSON("virtual_live_rewards", []any{}).Optional(),
		field.JSON("virtual_live_cheer_point_rewards", []any{}).Optional(),
		field.JSON("virtual_live_waiting_room", map[string]any{}).Optional(),
		field.JSON("virtual_items", []any{}).Optional(),
		field.JSON("virtual_live_appeals", []any{}).Optional(),
		field.JSON("virtual_live_background_musics", []any{}).Optional(),
		field.JSON("virtual_live_information", map[string]any{}).Optional(),
		field.Int64("archive_release_condition_id").Optional(),
		field.Int64("sub_game_character_penlight_color_group_id").Optional(),
		field.Int64("virtual_live_group_id").Optional(),
	}
}

// Edges of the Virtuallive.
func (Virtuallive) Edges() []ent.Edge {
	return nil
}

// Indexes of the Virtuallive.
func (Virtuallive) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
