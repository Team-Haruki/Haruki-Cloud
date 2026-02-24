package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaifixturegamecharactergroupperformancebonuse holds the schema definition for the Mysekaifixturegamecharactergroupperformancebonuse entity.
type Mysekaifixturegamecharactergroupperformancebonuse struct {
	ent.Schema
}

// Fields of the Mysekaifixturegamecharactergroupperformancebonuse.
func (Mysekaifixturegamecharactergroupperformancebonuse) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("mysekai_fixture_game_character_group_id").Optional(),
		field.Int64("bonus_rate").Optional(),
	}
}

// Edges of the Mysekaifixturegamecharactergroupperformancebonuse.
func (Mysekaifixturegamecharactergroupperformancebonuse) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaifixturegamecharactergroupperformancebonuse.
func (Mysekaifixturegamecharactergroupperformancebonuse) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
