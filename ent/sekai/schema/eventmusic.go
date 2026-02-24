package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventmusic holds the schema definition for the Eventmusic entity.
type Eventmusic struct {
	ent.Schema
}

// Fields of the Eventmusic.
func (Eventmusic) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("event_id").Optional(),
		field.Int64("music_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("release_condition_id").Optional(),
	}
}

// Edges of the Eventmusic.
func (Eventmusic) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventmusic.
func (Eventmusic) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("event_id", "music_id").Fields("server_region"),
	}
}
