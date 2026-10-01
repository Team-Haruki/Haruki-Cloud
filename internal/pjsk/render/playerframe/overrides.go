package playerframe

import (
	"context"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

// Overrides holds the file-configured account frames (pjsk_render.player_frame_overrides),
// keyed by server and actual game UID. It is built once at startup and only read afterwards.
type Overrides map[string]overrideParts

// overrideParts is one account's sprites per cell; vertical is optional.
type overrideParts struct {
	horizontal drawing.PlayerFrameParts
	vertical   *drawing.PlayerFrameParts
}

// NewOverrides copies the configured entries so later edits to the config slice cannot
// reach rendering.
func NewOverrides(entries []config.PlayerFrameOverride) Overrides {
	out := make(Overrides, len(entries))
	for _, e := range entries {
		parts := overrideParts{horizontal: drawingParts(e.Horizontal)}
		if e.Vertical != nil {
			parts.vertical = new(drawingParts(*e.Vertical))
		}
		out[overrideKey(renderregion.Normalize(e.Server), e.UserID)] = parts
	}
	return out
}

func drawingParts(p config.PlayerFrameParts) drawing.PlayerFrameParts {
	return drawing.PlayerFrameParts{
		Base: p.Base, CenterTop: p.CenterTop, LeftTop: p.LeftTop,
		RightTop: p.RightTop, LeftBottom: p.LeftBottom, RightBottom: p.RightBottom,
	}
}

func overrideKey(region renderregion.Value, userID string) string {
	return region.String() + ":" + strings.TrimSpace(userID)
}

// Lookup returns a fresh copy of the override for one account, so a request that edits
// its paths never touches the shared configuration.
func (o Overrides) Lookup(region renderregion.Value, userID string) (*drawing.PlayerFramePaths, bool) {
	entry, ok := o[overrideKey(region, userID)]
	if !ok {
		return nil, false
	}
	parts := entry.horizontal
	paths := &drawing.PlayerFramePaths{
		FrameType: "single", Base: parts.Base, CenterTop: parts.CenterTop,
		LeftTop: parts.LeftTop, RightTop: parts.RightTop,
		LeftBottom: parts.LeftBottom, RightBottom: parts.RightBottom,
		Horizontal: new(parts),
	}
	if entry.vertical != nil {
		paths.Vertical = new(*entry.vertical)
	}
	return paths, true
}

// ResolveAccount is the one place an account's displayed frame is decided: a file override
// for (region, uid) wins, otherwise the frame the account has equipped in game. Every
// profile builder — live, snapshot, modular and the snapshot-backed info panel used by
// card, deck, education, mysekai and the other commands — must come through here, or the
// same account renders with a frame in one command and without it in another.
func ResolveAccount(source Source, overrides Overrides, region renderregion.Value, userID string, frames []UserFrame) *drawing.PlayerFramePaths {
	if paths, ok := overrides.Lookup(region, userID); ok {
		return paths
	}
	if source == nil {
		return nil
	}
	return Build(source, frames)
}

// ResolveAccountFromProvider is ResolveAccount over a masterdata provider.
func ResolveAccountFromProvider(ctx context.Context, p provider.PlayerFrameProvider, overrides Overrides, region renderregion.Value, userID string, frames []UserFrame) *drawing.PlayerFramePaths {
	var source Source
	if p != nil {
		source = providerSource{ctx: ctx, p: p, region: region}
	}
	return ResolveAccount(source, overrides, region, userID, frames)
}
