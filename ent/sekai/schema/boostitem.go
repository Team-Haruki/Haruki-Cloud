package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Boostitem holds the schema definition for the Boostitem entity.
type Boostitem struct {
	ent.Schema
}

// Fields of the Boostitem.
func (Boostitem) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.String("name").Optional(),
		field.Int64("recovery_value").Optional(),
		field.String("asset_bundle_name").Optional(),
		field.String("flavor_text").Optional(),
	}
}

// Edges of the Boostitem.
func (Boostitem) Edges() []ent.Edge {
	return nil
}

// Indexes of the Boostitem.
func (Boostitem) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
