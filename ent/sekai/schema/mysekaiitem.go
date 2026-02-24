package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaiitem holds the schema definition for the Mysekaiitem entity.
type Mysekaiitem struct {
	ent.Schema
}

// Fields of the Mysekaiitem.
func (Mysekaiitem) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.String("mysekai_item_type").Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
		field.String("description").Optional(),
		field.String("icon_assetbundle_name").Optional(),
	}
}

// Edges of the Mysekaiitem.
func (Mysekaiitem) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaiitem.
func (Mysekaiitem) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
