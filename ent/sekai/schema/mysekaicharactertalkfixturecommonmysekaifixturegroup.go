package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Mysekaicharactertalkfixturecommonmysekaifixturegroup holds the schema definition for the Mysekaicharactertalkfixturecommonmysekaifixturegroup entity.
type Mysekaicharactertalkfixturecommonmysekaifixturegroup struct {
	ent.Schema
}

// Fields of the Mysekaicharactertalkfixturecommonmysekaifixturegroup.
func (Mysekaicharactertalkfixturecommonmysekaifixturegroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("group_id").Optional(),
		field.Int64("mysekai_fixture_id").Optional(),
	}
}

// Edges of the Mysekaicharactertalkfixturecommonmysekaifixturegroup.
func (Mysekaicharactertalkfixturecommonmysekaifixturegroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Mysekaicharactertalkfixturecommonmysekaifixturegroup.
func (Mysekaicharactertalkfixturecommonmysekaifixturegroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
