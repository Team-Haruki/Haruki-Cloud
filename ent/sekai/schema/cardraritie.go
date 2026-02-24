package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cardraritie holds the schema definition for the Cardraritie entity.
type Cardraritie struct {
	ent.Schema
}

// Fields of the Cardraritie.
func (Cardraritie) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.String("card_rarity_type").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("max_level").Optional(),
		field.Int64("max_skill_level").Optional(),
		field.Int64("training_max_level").Optional(),
	}
}

// Edges of the Cardraritie.
func (Cardraritie) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cardraritie.
func (Cardraritie) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("card_rarity_type", "seq").Fields("server_region"),
	}
}
