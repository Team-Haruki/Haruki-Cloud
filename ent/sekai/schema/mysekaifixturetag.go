package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixturetag holds the schema definition for the Mysekaifixturetag entity.
type Mysekaifixturetag struct {
	ent.Schema
}

// Fields of the Mysekaifixturetag.
func (Mysekaifixturetag) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
		field.JSON("mysekai_fixture_tag_type", map[string]any{}).Optional(),
		field.Int64("external_id").Optional(),
	}
}

// Edges of the Mysekaifixturetag.
func (Mysekaifixturetag) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixturetag.
func (Mysekaifixturetag) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
