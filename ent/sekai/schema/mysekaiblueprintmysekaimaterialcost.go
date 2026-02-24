package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaiblueprintmysekaimaterialcost holds the schema definition for the Mysekaiblueprintmysekaimaterialcost entity.
type Mysekaiblueprintmysekaimaterialcost struct {
	ent.Schema
}

// Fields of the Mysekaiblueprintmysekaimaterialcost.
func (Mysekaiblueprintmysekaimaterialcost) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("mysekai_blueprint_id").Optional(),
		field.Int64("mysekai_material_id").Optional(),
		field.Int64("seq").Optional(),
		field.Int64("quantity").Optional(),
		field.JSON("mysekai_blueprint_type", map[string]any{}).Optional(),
	}
}

// Edges of the Mysekaiblueprintmysekaimaterialcost.
func (Mysekaiblueprintmysekaimaterialcost) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaiblueprintmysekaimaterialcost.
func (Mysekaiblueprintmysekaimaterialcost) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
