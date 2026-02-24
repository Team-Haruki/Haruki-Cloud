package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cardcostume3D holds the schema definition for the Cardcostume3D entity.
type Cardcostume3D struct {
	ent.Schema
}

// Fields of the Cardcostume3D.
func (Cardcostume3D) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("card_id").Optional(),
		field.Int64("costume3_d_id").Optional(),
		field.Bool("is_initial_obtain_hair").Optional(),
	}
}

// Edges of the Cardcostume3D.
func (Cardcostume3D) Edges() []ent.Edge {
	return nil
}

// Indexes of the Cardcostume3D.
func (Cardcostume3D) Indexes() []ent.Index {
	return []ent.Index{
		// Custom index
		index.Fields("card_id", "costume3_d_id").Fields("server_region"),
	}
}
