package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Event holds the schema definition for the Event entity.
type Event struct {
	ent.Schema
}

// Fields of the Event.
func (Event) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("event_type").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("bgm_assetbundle_name").Optional(),
		field.Int64("event_only_component_display_start_at").Optional(),
		field.Int64("start_at").Optional(),
		field.Int64("aggregate_at").Optional(),
		field.Int64("ranking_announce_at").Optional(),
		field.Int64("distribution_start_at").Optional(),
		field.Int64("event_only_component_display_end_at").Optional(),
		field.Int64("closed_at").Optional(),
		field.Int64("distribution_end_at").Optional(),
		field.Int64("virtual_live_id").Optional(),
		field.String("unit").Optional(),
		field.Bool("is_count_leader_character_play").Optional(),
		field.JSON("event_ranking_reward_ranges", []any{}).Optional(),
		field.String("event_point_assetbundle_name").Optional(),
		field.Int64("standby_screen_display_start_at").Optional(),
	}
}

// Edges of the Event.
func (Event) Edges() []ent.Edge {
	return nil
}

// Indexes of the Event.
func (Event) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
