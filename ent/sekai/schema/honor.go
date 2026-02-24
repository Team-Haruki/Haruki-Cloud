package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Honor holds the schema definition for the Honor entity.
type Honor struct {
	ent.Schema
}

// Fields of the Honor.
func (Honor) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("group_id").Optional(),
		field.String("honor_rarity").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
		field.JSON("levels", []any{}).Optional(),
		field.Int64("honor_type_id").Optional(),
		field.String("honor_mission_type").Optional(),
		field.Int64("start_at").Optional(),
	}
}

// Edges of the Honor.
func (Honor) Edges() []ent.Edge {
	return nil
}

// Indexes of the Honor.
func (Honor) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
