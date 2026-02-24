package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixturemaingenre holds the schema definition for the Mysekaifixturemaingenre entity.
type Mysekaifixturemaingenre struct {
	ent.Schema
}

// Fields of the Mysekaifixturemaingenre.
func (Mysekaifixturemaingenre) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.JSON("mysekai_fixture_main_genre_type", map[string]any{}).Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("group_id").Optional(),
	}
}

// Edges of the Mysekaifixturemaingenre.
func (Mysekaifixturemaingenre) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixturemaingenre.
func (Mysekaifixturemaingenre) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
