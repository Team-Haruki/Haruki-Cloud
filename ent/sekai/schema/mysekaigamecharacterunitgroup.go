package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaigamecharacterunitgroup holds the schema definition for the Mysekaigamecharacterunitgroup entity.
type Mysekaigamecharacterunitgroup struct {
	ent.Schema
}

// Fields of the Mysekaigamecharacterunitgroup.
func (Mysekaigamecharacterunitgroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("game_character_unit_id1").Optional(),
		field.Int64("game_character_unit_id2").Optional(),
		field.Int64("game_character_unit_id3").Optional(),
		field.Int64("game_character_unit_id4").Optional(),
		field.Int64("game_character_unit_id5").Optional(),
	}
}

// Edges of the Mysekaigamecharacterunitgroup.
func (Mysekaigamecharacterunitgroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaigamecharacterunitgroup.
func (Mysekaigamecharacterunitgroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
