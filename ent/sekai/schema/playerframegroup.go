package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Playerframegroup holds the schema definition for the Playerframegroup entity.
type Playerframegroup struct {
	ent.Schema
}

// Fields of the Playerframegroup.
func (Playerframegroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Playerframegroup.
func (Playerframegroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Playerframegroup.
func (Playerframegroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
