package profile

import "haruki-cloud/utils/drawing"

type Query struct {
	UserID string `json:"user_id,omitempty"`
	Region string `json:"region"`
	// Visible reflects the binding's visibility setting (!Visible → IsHideUID = true).
	Visible    bool                       `json:"visible"`
	BgSettings *drawing.ProfileBgSettings `json:"bg_settings,omitempty"`
}
