package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cheerfulcarnivalteam holds the schema definition for the Cheerfulcarnivalteam entity.
type Cheerfulcarnivalteam struct {
	ent.Schema
}

// Fields of the Cheerfulcarnivalteam.
func (Cheerfulcarnivalteam) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("event_id").Optional(),
		field.Int64("seq").Optional(),
		field.String("team_name").Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Cheerfulcarnivalteam.
func (Cheerfulcarnivalteam) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cheerfulcarnivalteam.
func (Cheerfulcarnivalteam) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
