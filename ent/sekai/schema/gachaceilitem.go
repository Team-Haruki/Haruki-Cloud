package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Gachaceilitem holds the schema definition for the Gachaceilitem entity.
type Gachaceilitem struct {
	ent.Schema
}

// Fields of the Gachaceilitem.
func (Gachaceilitem) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("gacha_id").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("convert_start_at").Optional(),
		field.Int64("convert_resource_box_id").Optional(),
	}
}

// Edges of the Gachaceilitem.
func (Gachaceilitem) Edges() []ent.Edge {
	return nil
}

// Indexes of the Gachaceilitem.
func (Gachaceilitem) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
