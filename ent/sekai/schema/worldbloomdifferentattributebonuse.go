package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Worldbloomdifferentattributebonuse holds the schema definition for the Worldbloomdifferentattributebonuse entity.
type Worldbloomdifferentattributebonuse struct {
	ent.Schema
}

// Fields of the Worldbloomdifferentattributebonuse.
func (Worldbloomdifferentattributebonuse) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("attribute_count").Optional(),
		field.Float("bonus_rate").Optional(),
	}
}

// Edges of the Worldbloomdifferentattributebonuse.
func (Worldbloomdifferentattributebonuse) Edges() []ent.Edge {
	return nil
}

// Indexes of the Worldbloomdifferentattributebonuse.
func (Worldbloomdifferentattributebonuse) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("attribute_count").Fields("server_region"),
	}
}
