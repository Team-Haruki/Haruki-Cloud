package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Stamp holds the schema definition for the Stamp entity.
type Stamp struct {
	ent.Schema
}

// Fields of the Stamp.
func (Stamp) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("stamp_type").Optional(),
		field.Int64("seq").Optional(),
		field.String("name").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("balloon_assetbundle_name").Optional(),
		field.Int64("character_id1").Optional(),
		field.Int64("game_character_unit_id").Optional(),
		field.Int64("archive_published_at").Optional(),
		field.String("description").Optional(),
		field.String("archive_display_type").Optional(),
		field.Int64("character_id2").Optional(),
	}
}

// Edges of the Stamp.
func (Stamp) Edges() []ent.Edge {
	return nil
}

// Indexes of the Stamp.
func (Stamp) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
