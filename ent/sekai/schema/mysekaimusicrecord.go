package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaimusicrecord holds the schema definition for the Mysekaimusicrecord entity.
type Mysekaimusicrecord struct {
	ent.Schema
}

// Fields of the Mysekaimusicrecord.
func (Mysekaimusicrecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.JSON("mysekai_music_track_type", map[string]any{}).Optional(),
		field.Int64("external_id").Optional(),
	}
}

// Edges of the Mysekaimusicrecord.
func (Mysekaimusicrecord) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaimusicrecord.
func (Mysekaimusicrecord) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
