package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Music holds the schema definition for the Music entity.
type Music struct {
	ent.Schema
}

// Fields of the Music.
func (Music) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("release_condition_id").Optional(),
		field.JSON("categories", []any{}).Optional(),
		field.String("title").Optional(),
		field.String("pronunciation").Optional(),
		field.Int64("creator_artist_id").Optional(),
		field.String("lyricist").Optional(),
		field.String("composer").Optional(),
		field.String("arranger").Optional(),
		field.Int64("dancer_count").Optional(),
		field.Int64("self_dancer_position").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("live_talk_background_assetbundle_name").Optional(),
		field.Int64("published_at").Optional(),
		field.Int64("released_at").Optional(),
		field.Int64("live_stage_id").Optional(),
		field.Float("filler_sec").Optional(),
		field.Bool("is_newly_written_music").Optional(),
		field.Bool("is_full_length").Optional(),
		field.Int64("music_collaboration_id").Optional(),
		field.JSON("infos", []any{}).Optional(),
	}
}

// Edges of the Music.
func (Music) Edges() []ent.Edge {
	return nil
}

// Indexes of the Music.
func (Music) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
