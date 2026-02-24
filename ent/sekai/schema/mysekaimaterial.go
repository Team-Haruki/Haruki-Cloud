package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaimaterial holds the schema definition for the Mysekaimaterial entity.
type Mysekaimaterial struct {
	ent.Schema
}

// Fields of the Mysekaimaterial.
func (Mysekaimaterial) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.JSON("mysekai_material_type", map[string]any{}).Optional(),
		field.String("name").Optional(),
		field.String("pronunciation").Optional(),
		field.String("description").Optional(),
		field.JSON("mysekai_material_rarity_type", map[string]any{}).Optional(),
		field.String("icon_assetbundle_name").Optional(),
		field.String("model_assetbundle_name").Optional(),
		field.JSON("mysekai_site_ids", []any{}).Optional(),
		field.Int64("mysekai_phenomena_group_id").Optional(),
	}
}

// Edges of the Mysekaimaterial.
func (Mysekaimaterial) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaimaterial.
func (Mysekaimaterial) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
