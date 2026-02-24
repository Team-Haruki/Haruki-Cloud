package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MusicArtist holds the schema definition for the MusicArtist entity.
type MusicArtist struct {
	ent.Schema
}

// Fields of the MusicArtist.
func (MusicArtist) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
	}
}

// Edges of the MusicArtist.
func (MusicArtist) Edges() []ent.Edge {
	return nil
}

// Indexes of the MusicArtist.
func (MusicArtist) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
