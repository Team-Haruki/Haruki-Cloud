package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaicharactertalkconditiongroup holds the schema definition for the Mysekaicharactertalkconditiongroup entity.
type Mysekaicharactertalkconditiongroup struct {
	ent.Schema
}

// Fields of the Mysekaicharactertalkconditiongroup.
func (Mysekaicharactertalkconditiongroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("group_id").Optional(),
		field.Int64("mysekai_character_talk_condition_id").Optional(),
	}
}

// Edges of the Mysekaicharactertalkconditiongroup.
func (Mysekaicharactertalkconditiongroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaicharactertalkconditiongroup.
func (Mysekaicharactertalkconditiongroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
