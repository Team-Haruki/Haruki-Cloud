package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Musictag holds the schema definition for the Musictag entity.
type Musictag struct {
	ent.Schema
}

// Fields of the Musictag.
func (Musictag) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("music_id").Optional(),
		field.String("music_tag").Optional(),
		field.Int64("seq").Optional(),
	}
}

// Edges of the Musictag.
func (Musictag) Edges() []ent.Edge {
	return nil
}

// Indexes of the Musictag.
func (Musictag) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("music_id", "music_tag").Fields("server_region"),
	}
}
