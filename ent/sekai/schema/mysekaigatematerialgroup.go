package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaigatematerialgroup holds the schema definition for the Mysekaigatematerialgroup entity.
type Mysekaigatematerialgroup struct {
	ent.Schema
}

// Fields of the Mysekaigatematerialgroup.
func (Mysekaigatematerialgroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("group_id").Optional(),
		field.Int64("mysekai_material_id").Optional(),
		field.Int64("quantity").Optional(),
	}
}

// Edges of the Mysekaigatematerialgroup.
func (Mysekaigatematerialgroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaigatematerialgroup.
func (Mysekaigatematerialgroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
