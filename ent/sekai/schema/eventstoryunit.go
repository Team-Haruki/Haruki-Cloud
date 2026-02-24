package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventstoryunit holds the schema definition for the Eventstoryunit entity.
type Eventstoryunit struct {
	ent.Schema
}

// Fields of the Eventstoryunit.
func (Eventstoryunit) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("event_story_id").Optional(),
		field.String("unit").Optional(),
		field.String("event_story_unit_relation").Optional(),
	}
}

// Edges of the Eventstoryunit.
func (Eventstoryunit) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventstoryunit.
func (Eventstoryunit) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
