package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixtureonlydisassemblematerial holds the schema definition for the Mysekaifixtureonlydisassemblematerial entity.
type Mysekaifixtureonlydisassemblematerial struct {
	ent.Schema
}

// Fields of the Mysekaifixtureonlydisassemblematerial.
func (Mysekaifixtureonlydisassemblematerial) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("mysekai_fixture_id").Optional(),
		field.Int64("mysekai_material_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("quantity").Optional(),
	}
}

// Edges of the Mysekaifixtureonlydisassemblematerial.
func (Mysekaifixtureonlydisassemblematerial) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixtureonlydisassemblematerial.
func (Mysekaifixtureonlydisassemblematerial) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
