package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Area holds the schema definition for the Area entity.
type Area struct {
	ent.Schema
}

// Fields of the Area.
func (Area) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("assetbundle_name").Optional(),
		field.Int64("group_id").Optional(),
		field.Bool("is_base_area").Optional(),
		field.String("area_type").Optional(),
		field.String("view_type").Optional(),
		field.String("display_timeline_type").Optional(),
		field.String("additional_area_type").Optional(),
		field.String("name").Optional(),
		field.Int64("release_condition_id").Optional(),
		field.String("sub_name").Optional(),
		field.String("label").Optional(),
		field.Int64("start_at").Optional(),
		field.Int64("end_at").Optional(),
		field.Int64("release_condition_id2").Optional(),
	}
}

// Edges of the Area.
func (Area) Edges() []ent.Edge {
	return nil
}

// Indexes of the Area.
func (Area) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
