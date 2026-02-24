package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Characterrank holds the schema definition for the Characterrank entity.
type Characterrank struct {
	ent.Schema
}

// Fields of the Characterrank.
func (Characterrank) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("character_id").Optional(),
		field.Int64("character_rank").Optional(),
		field.Float("power1_bonus_rate").Optional(),
		field.Float("power2_bonus_rate").Optional(),
		field.Float("power3_bonus_rate").Optional(),
		field.JSON("reward_resource_box_ids", []any{}).Optional(),
		field.JSON("character_rank_achieve_resources", []any{}).Optional(),
	}
}

// Edges of the Characterrank.
func (Characterrank) Edges() []ent.Edge {
	return nil
}

// Indexes of the Characterrank.
func (Characterrank) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
