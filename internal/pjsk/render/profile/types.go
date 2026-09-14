package profile

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

// ── Data source ─────────────────────────────────────────────────────────────

type DataSource interface {
	honor.DataSource
	GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error)
	GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error)
	GetCardByID(id int) (*masterdata.Card, error)
	GetEventIDByHonorID(honorID int) int
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

// ── Controller ──────────────────────────────────────────────────────────────

// censorService is the subset of censor.Service used by the profile
// controller. Kept as an interface so tests can observe the concurrent
// name/bio moderation calls.
type censorService interface {
	CensorName(ctx context.Context, harukiUserID int, userID string, name string, server string) bool
	CensorShortBio(ctx context.Context, harukiUserID int, userID string, content string, server string) bool
}

type Controller struct {
	sources    *regionsource.Registry[DataSource]
	drawing    *drawing.HarukiDrawingClient
	assets     *assets.AssetHelper
	snapshot   snapshot.Snapshot
	censor     censorService
	requestCtx context.Context
}

// ── Query ───────────────────────────────────────────────────────────────────

type Query struct {
	UserID string `json:"user_id,omitempty"`
	Region string `json:"region"`
	// Visible reflects the binding's visibility setting (!Visible → IsHideUID = true).
	Visible          bool                       `json:"visible"`
	BgSettings       *drawing.ProfileBgSettings `json:"bg_settings,omitempty"`
	VerticalOverride *bool                      `json:"vertical_override,omitempty"`
}
