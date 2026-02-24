package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixturesubgenre holds the schema definition for the Mysekaifixturesubgenre entity.
type Mysekaifixturesubgenre struct {
	ent.Schema
}

// Fields of the Mysekaifixturesubgenre.
func (Mysekaifixturesubgenre) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.String("mysekai_fixture_sub_genre_type").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Mysekaifixturesubgenre.
func (Mysekaifixturesubgenre) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixturesubgenre.
func (Mysekaifixturesubgenre) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
