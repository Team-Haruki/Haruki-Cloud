package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Honorgroup holds the schema definition for the Honorgroup entity.
type Honorgroup struct {
	ent.Schema
}

// Fields of the Honorgroup.
func (Honorgroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
		field.String("honor_type").Optional(),
		field.String("background_assetbundle_name").Optional(),
		field.String("frame_name").Optional(),
	}
}

// Edges of the Honorgroup.
func (Honorgroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Honorgroup.
func (Honorgroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
