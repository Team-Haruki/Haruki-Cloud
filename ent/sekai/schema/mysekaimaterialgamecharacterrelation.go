package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaimaterialgamecharacterrelation holds the schema definition for the Mysekaimaterialgamecharacterrelation entity.
type Mysekaimaterialgamecharacterrelation struct {
	ent.Schema
}

// Fields of the Mysekaimaterialgamecharacterrelation.
func (Mysekaimaterialgamecharacterrelation) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("group_id").Optional(),
		field.Int64("mysekai_material_id").Optional(),
		field.Int64("game_character_id").Optional(),
	}
}

// Edges of the Mysekaimaterialgamecharacterrelation.
func (Mysekaimaterialgamecharacterrelation) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaimaterialgamecharacterrelation.
func (Mysekaimaterialgamecharacterrelation) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
