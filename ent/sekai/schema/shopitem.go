package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Shopitem holds the schema definition for the Shopitem entity.
type Shopitem struct {
	ent.Schema
}

// Fields of the Shopitem.
func (Shopitem) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("shop_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("release_condition_id").Optional(),
		field.Int64("resource_box_id").Optional(),
		field.JSON("costs", []any{}).Optional(),
		field.Int64("start_at").Optional(),
	}
}

// Edges of the Shopitem.
func (Shopitem) Edges() []ent.Edge {
	return nil
}

// Indexes of the Shopitem.
func (Shopitem) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
