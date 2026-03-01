package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaiphenomenon holds the schema definition for the Mysekaiphenomenon entity.
type Mysekaiphenomenon struct {
	ent.Schema
}

// Fields of the Mysekaiphenomenon.
func (Mysekaiphenomenon) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.JSON("mysekai_phenomena_brightness_type", map[string]any{}).Optional(),
		field.String("name").Optional(),
		field.String("english_name").Optional(),
		field.String("description").Optional(),
		field.JSON("mysekai_phenomena_time_period_type", map[string]any{}).Optional(),
		field.Int64("mysekai_phenomena_background_color_id").Optional(),
		field.String("assetbundle_name").Optional(),
		field.String("ramp_texture_assetbundle_name").Optional(),
		field.String("icon_assetbundle_name").Optional(),
	}
}

// Edges of the Mysekaiphenomenon.
func (Mysekaiphenomenon) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaiphenomenon.
func (Mysekaiphenomenon) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
