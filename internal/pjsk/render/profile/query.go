package profile

type Query struct {
	UserID string `json:"user_id,omitempty"`
	Region string `json:"region"`
	// Visible reflects the binding's visibility setting (!Visible → IsHideUID = true).
	Visible bool `json:"visible"`
}
