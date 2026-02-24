package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaimusicrecordcategorie holds the schema definition for the Mysekaimusicrecordcategorie entity.
type Mysekaimusicrecordcategorie struct {
	ent.Schema
}

// Fields of the Mysekaimusicrecordcategorie.
func (Mysekaimusicrecordcategorie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.Int64("seq").Optional(),
		field.JSON("mysekai_music_track_type", map[string]any{}).Optional(),
		field.String("unit").Optional(),
	}
}

// Edges of the Mysekaimusicrecordcategorie.
func (Mysekaimusicrecordcategorie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaimusicrecordcategorie.
func (Mysekaimusicrecordcategorie) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
