package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Limitedtimemusic holds the schema definition for the Limitedtimemusic entity.
type Limitedtimemusic struct {
	ent.Schema
}

// Fields of the Limitedtimemusic.
func (Limitedtimemusic) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("music_id").Optional(),
		field.Int64("start_at").Optional(),
		field.Int64("end_at").Optional(),
	}
}

// Edges of the Limitedtimemusic.
func (Limitedtimemusic) Edges() []ent.Edge {
	return nil
}

// Indexes of the Limitedtimemusic.
func (Limitedtimemusic) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
