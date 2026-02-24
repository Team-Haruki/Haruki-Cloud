package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Eventraritybonusrate holds the schema definition for the Eventraritybonusrate entity.
type Eventraritybonusrate struct {
	ent.Schema
}

// Fields of the Eventraritybonusrate.
func (Eventraritybonusrate) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.String("card_rarity_type").Optional(),
		field.Int64("master_rank").Optional(),
		field.Float("bonus_rate").Optional(),
	}
}

// Edges of the Eventraritybonusrate.
func (Eventraritybonusrate) Edges() []ent.Edge {
	return nil
}

// Indexes of the Eventraritybonusrate.
func (Eventraritybonusrate) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
