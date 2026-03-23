package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"haruki-cloud/utils/sekai"
)

// UserSettings holds all user-configurable preferences in a single JSONB value.
// Adding new settings is backward-compatible: existing rows will deserialize
// missing fields as zero values / Go defaults.
type UserSettings struct {
	// PJSKEnabledDifficulties controls which music difficulties are shown in
	// arrest/clear-stats output. Defaults to [expert, master].
	PJSKEnabledDifficulties []sekai.MusicDifficultyType `json:"pjsk_enabled_difficulties,omitempty"`
}

type UserPreference struct {
	ent.Schema
}

func (UserPreference) Fields() []ent.Field {
	return []ent.Field{
		field.Int("haruki_user_id").Unique().Comment("Reference to users table"),
		field.JSON("settings", &UserSettings{}).
			Default(func() *UserSettings {
				return &UserSettings{
					PJSKEnabledDifficulties: []sekai.MusicDifficultyType{
						sekai.MusicDifficultyExpert,
						sekai.MusicDifficultyMaster,
					},
				}
			}).
			Comment("User settings stored as JSONB"),
	}
}

func (UserPreference) Indexes() []ent.Index {
	return nil
}

func (UserPreference) Edges() []ent.Edge {
	return nil
}
