package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Challengelivehighscorereward holds the schema definition for the Challengelivehighscorereward entity.
type Challengelivehighscorereward struct {
	ent.Schema
}

// Fields of the Challengelivehighscorereward.
func (Challengelivehighscorereward) Fields() []ent.Field {
	return []ent.Field{
		field.String("server_region"),
		field.Int64("game_id").Optional(),
		field.Int64("character_id").Optional(),
		field.Int64("high_score").Optional(),
		field.Int64("resource_box_id").Optional(),
	}
}

// Edges of the Challengelivehighscorereward.
func (Challengelivehighscorereward) Edges() []ent.Edge {
	return nil
}

// Indexes of the Challengelivehighscorereward.
func (Challengelivehighscorereward) Indexes() []ent.Index {
	return []ent.Index{
		// Index for game_id and region
		index.Fields("game_id", "server_region"),
	}
}
