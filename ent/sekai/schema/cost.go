package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Cost holds the schema definition for the Cost entity.
type Cost struct {
	ent.Schema
}

// Fields of the Cost.
func (Cost) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("shop_item_id").Optional(),
		field.Int64("seq").Optional(),
		field.JSON("cost", map[string]any{}).Optional(),
	}
}

// Edges of the Cost.
func (Cost) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cost.
func (Cost) Indexes() []ent.Index {
	return []ent.Index{
		// No index generated
	}
}
