package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Areaitem holds the schema definition for the Areaitem entity.
type Areaitem struct {
	ent.Schema
}

// Fields of the Areaitem.
func (Areaitem) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("area_id").Optional(),
		field.String("name").Optional(),
		field.String("flavor_text").Optional(),
		field.String("spawn_point").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Areaitem.
func (Areaitem) Edges() []ent.Edge {
	return nil
}

// Indexes of the Areaitem.
func (Areaitem) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
