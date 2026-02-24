package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Costume3D holds the schema definition for the Costume3D entity.
type Costume3D struct {
	ent.Schema
}

// Fields of the Costume3D.
func (Costume3D) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("costume3_d_group_id").Optional(),
		field.String("costume3_d_type").Optional(),
		field.String("name").Optional(),
		field.String("part_type").Optional(),
		field.Int64("color_id").Optional(),
		field.String("color_name").Optional(),
		field.Int64("character_id").Optional(),
		field.String("costume3_d_rarity").Optional(),
		field.String("how_to_obtain").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("designer").Optional(),
		field.String("archive_display_type").Optional(),
		field.Int64("archive_published_at").Optional(),
		field.Int64("published_at").Optional(),
	}
}

// Edges of the Costume3D.
func (Costume3D) Edges() []ent.Edge {
	return nil
}

// Indexes of the Costume3D.
func (Costume3D) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
