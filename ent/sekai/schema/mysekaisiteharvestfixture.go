package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaisiteharvestfixture holds the schema definition for the Mysekaisiteharvestfixture entity.
type Mysekaisiteharvestfixture struct {
	ent.Schema
}

// Fields of the Mysekaisiteharvestfixture.
func (Mysekaisiteharvestfixture) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("mysekai_site_harvest_fixture_type").Optional(),
		field.Int64("hp").Optional(),
		field.Int64("last_attack_stamina").Optional(),
		field.JSON("mysekai_site_harvest_fixture_rarity_type", map[string]any{}).Optional(),
		field.String("assetbundle_name").Optional(),
	}
}

// Edges of the Mysekaisiteharvestfixture.
func (Mysekaisiteharvestfixture) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaisiteharvestfixture.
func (Mysekaisiteharvestfixture) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
