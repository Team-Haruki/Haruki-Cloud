package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Characterarchivemysekaicharactertalkgroup holds the schema definition for the Characterarchivemysekaicharactertalkgroup entity.
type Characterarchivemysekaicharactertalkgroup struct {
	ent.Schema
}

// Fields of the Characterarchivemysekaicharactertalkgroup.
func (Characterarchivemysekaicharactertalkgroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("seq").Optional(),
		field.String("archive_display_type").Optional(),
	}
}

// Edges of the Characterarchivemysekaicharactertalkgroup.
func (Characterarchivemysekaicharactertalkgroup) Edges() []ent.Edge {
	return nil
}

// Indexes of the Characterarchivemysekaicharactertalkgroup.
func (Characterarchivemysekaicharactertalkgroup) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
