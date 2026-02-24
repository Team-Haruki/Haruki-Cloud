package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Resourceboxe holds the schema definition for the Resourceboxe entity.
type Resourceboxe struct {
	ent.Schema
}

// Fields of the Resourceboxe.
func (Resourceboxe) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.String("resource_box_purpose").Optional(),
		field.Int64("game_id").Optional(),
		field.String("resource_box_type").Optional(),
		field.String("description").Optional(),
		field.JSON("details", []any{}).Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Resourceboxe.
func (Resourceboxe) Edges() []ent.Edge {
	return nil
}

// Indexes of the Resourceboxe.
func (Resourceboxe) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
