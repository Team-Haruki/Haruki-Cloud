package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Bondshonor holds the schema definition for the Bondshonor entity.
type Bondshonor struct {
	ent.Schema
}

// Fields of the Bondshonor.
func (Bondshonor) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("bonds_group_id").Optional(),
		field.Int64("game_character_unit_id1").Optional(),
		field.Int64("game_character_unit_id2").Optional(),
		field.String("honor_rarity").Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
		field.String("description").Optional(),
		field.JSON("levels", []any{}).Optional(),
		field.Bool("configurable_unit_virtual_singer").Optional(),
	}
}

// Edges of the Bondshonor.
func (Bondshonor) Edges() []ent.Edge {
	return nil
}

// Indexes of the Bondshonor.
func (Bondshonor) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
