package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cardepisode holds the schema definition for the Cardepisode entity.
type Cardepisode struct {
	ent.Schema
}

// Fields of the Cardepisode.
func (Cardepisode) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("card_id").Optional(),
		field.String("title").Optional(),
		field.String("scenario_id").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("release_condition_id").Optional(),
		field.Int64("power1_bonus_fixed").Optional(),
		field.Int64("power2_bonus_fixed").Optional(),
		field.Int64("power3_bonus_fixed").Optional(),
		field.JSON("reward_resource_box_ids", []any{}).Optional(),
		field.JSON("costs", []any{}).Optional(),
		field.String("card_episode_part_type").Optional(),
	}
}

// Edges of the Cardepisode.
func (Cardepisode) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cardepisode.
func (Cardepisode) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
