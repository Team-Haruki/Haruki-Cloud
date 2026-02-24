package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Charactermissionv2Parametergroup holds the schema definition for the Charactermissionv2Parametergroup entity.
type Charactermissionv2Parametergroup struct {
	ent.Schema
}

// Fields of the Charactermissionv2Parametergroup.
func (Charactermissionv2Parametergroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("requirement").Optional(),
		field.Int64("exp").Optional(),
		field.Int64("quantity").Optional(),
	}
}

// Edges of the Charactermissionv2Parametergroup.
func (Charactermissionv2Parametergroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Charactermissionv2Parametergroup.
func (Charactermissionv2Parametergroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
