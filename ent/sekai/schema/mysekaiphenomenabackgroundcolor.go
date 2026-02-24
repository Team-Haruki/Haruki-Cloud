package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaiphenomenabackgroundcolor holds the schema definition for the Mysekaiphenomenabackgroundcolor entity.
type Mysekaiphenomenabackgroundcolor struct {
	ent.Schema
}

// Fields of the Mysekaiphenomenabackgroundcolor.
func (Mysekaiphenomenabackgroundcolor) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("base_color").Optional(),
		field.String("ground_color").Optional(),
		field.String("gradation_color").Optional(),
		field.String("corner_color").Optional(),
		field.String("ground_highlight_color").Optional(),
	}
}

// Edges of the Mysekaiphenomenabackgroundcolor.
func (Mysekaiphenomenabackgroundcolor) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaiphenomenabackgroundcolor.
func (Mysekaiphenomenabackgroundcolor) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
