package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Gachaticket holds the schema definition for the Gachaticket entity.
type Gachaticket struct {
	ent.Schema
}

// Fields of the Gachaticket.
func (Gachaticket) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("gacha_display_type").Optional(),
	}
}

// Edges of the Gachaticket.
func (Gachaticket) Edges() []ent.Edge {
	return nil
}

// Indexes of the Gachaticket.
func (Gachaticket) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
