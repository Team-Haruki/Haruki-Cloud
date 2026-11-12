package playerframe

import (
	"context"
	"path"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// Preferences resolves an account's decoration after its real server and UID
// have been established. It must never key preferences by the requesting QQ.
type Preferences interface {
	Lookup(context.Context, renderregion.Value, string) *drawing.PlayerFramePaths
}

type frameClient interface {
	GetSponsorFrameContext(context.Context, string, string) (*sekaiapi.SponsorFrame, error)
}

type preferenceEntry struct {
	frame   *sekaiapi.SponsorFrame
	expires time.Time
}

// ToolboxPreferences bounds both latency and memory. A short TTL also bounds
// how long a change, unlink, ban or ownership transfer takes to reach images.
// Outages fall back to the existing operator/game frame, never fail rendering.
type ToolboxPreferences struct {
	client frameClient
	mu     sync.Mutex
	cache  map[string]preferenceEntry
}

func NewToolboxPreferences(client frameClient) *ToolboxPreferences {
	return &ToolboxPreferences{client: client, cache: make(map[string]preferenceEntry)}
}

func (p *ToolboxPreferences) Lookup(ctx context.Context, region renderregion.Value, uid string) *drawing.PlayerFramePaths {
	if p == nil || p.client == nil || uid == "" {
		return nil
	}
	key := overrideKey(region, uid)
	p.mu.Lock()
	entry, exists := p.cache[key]
	p.mu.Unlock()
	if exists && time.Now().Before(entry.expires) {
		return preferencePaths(entry.frame)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	frame, err := p.client.GetSponsorFrameContext(requestCtx, region.String(), uid)
	if err != nil {
		frame = nil
	}
	// Store a value copy only. Each returned Drawing model is independent.
	if frame != nil {
		frame = new(*frame)
	}
	p.mu.Lock()
	if len(p.cache) >= 2048 {
		for k := range p.cache {
			delete(p.cache, k)
			break
		}
	}
	p.cache[key] = preferenceEntry{frame: frame, expires: time.Now().Add(15 * time.Second)}
	p.mu.Unlock()
	return preferencePaths(frame)
}

func preferenceParts(p sekaiapi.SponsorFrameParts) (drawing.PlayerFrameParts, bool) {
	for _, value := range []string{p.Base, p.CenterTop, p.LeftTop, p.RightTop, p.LeftBottom, p.RightBottom} {
		if !strings.HasPrefix(value, "static_images/custom_frames/") || path.Clean(value) != value ||
			strings.ContainsAny(value, "\\\x00\r\n") || !strings.HasSuffix(value, ".png") {
			return drawing.PlayerFrameParts{}, false
		}
	}
	return drawing.PlayerFrameParts{Base: p.Base, CenterTop: p.CenterTop, LeftTop: p.LeftTop, RightTop: p.RightTop, LeftBottom: p.LeftBottom, RightBottom: p.RightBottom}, true
}

func preferencePaths(frame *sekaiapi.SponsorFrame) *drawing.PlayerFramePaths {
	if frame == nil {
		return nil
	}
	h, ok := preferenceParts(frame.Horizontal)
	if !ok {
		return nil
	}
	v, ok := preferenceParts(frame.Vertical)
	if !ok {
		return nil
	}
	return &drawing.PlayerFramePaths{FrameType: "single", Base: h.Base, CenterTop: h.CenterTop, LeftTop: h.LeftTop, RightTop: h.RightTop,
		LeftBottom: h.LeftBottom, RightBottom: h.RightBottom, Horizontal: &h, Vertical: &v}
}

func ResolvePreferredAccount(ctx context.Context, preferences Preferences, source Source, overrides Overrides, region renderregion.Value, userID string, frames []UserFrame) *drawing.PlayerFramePaths {
	if preferences != nil {
		if selected := preferences.Lookup(ctx, region, userID); selected != nil {
			return selected
		}
	}
	return ResolveAccount(source, overrides, region, userID, frames)
}
