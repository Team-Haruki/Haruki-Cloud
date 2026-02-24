package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixturegamecharactergroup holds the schema definition for the Mysekaifixturegamecharactergroup entity.
type Mysekaifixturegamecharactergroup struct {
	ent.Schema
}

// Fields of the Mysekaifixturegamecharactergroup.
func (Mysekaifixturegamecharactergroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("group_id").Optional(),
		field.Int64("game_character_id").Optional(),
	}
}

// Edges of the Mysekaifixturegamecharactergroup.
func (Mysekaifixturegamecharactergroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixturegamecharactergroup.
func (Mysekaifixturegamecharactergroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
