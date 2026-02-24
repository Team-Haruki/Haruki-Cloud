package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaicharactertalkcondition holds the schema definition for the Mysekaicharactertalkcondition entity.
type Mysekaicharactertalkcondition struct {
	ent.Schema
}

// Fields of the Mysekaicharactertalkcondition.
func (Mysekaicharactertalkcondition) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.JSON("mysekai_character_talk_condition_type", map[string]any{}).Optional(),
		field.Int64("mysekai_character_talk_condition_type_value").Optional(),
	}
}

// Edges of the Mysekaicharactertalkcondition.
func (Mysekaicharactertalkcondition) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaicharactertalkcondition.
func (Mysekaicharactertalkcondition) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
