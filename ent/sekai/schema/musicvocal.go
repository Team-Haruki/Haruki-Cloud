package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Musicvocal holds the schema definition for the Musicvocal entity.
type Musicvocal struct {
	ent.Schema
}

// Fields of the Musicvocal.
func (Musicvocal) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("music_id").Optional(),
		field.String("music_vocal_type").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("release_condition_id").Optional(),
		field.String("caption").Optional(),
		field.JSON("characters", []any{}).Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("archive_published_at").Optional(),
		field.Int64("special_season_id").Optional(),
		field.String("archive_display_type").Optional(),
	}
}

// Edges of the Musicvocal.
func (Musicvocal) Edges() []ent.Edge {
	return nil
}

// Indexes of the Musicvocal.
func (Musicvocal) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
