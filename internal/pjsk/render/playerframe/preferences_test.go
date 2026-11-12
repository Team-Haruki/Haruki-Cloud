package playerframe

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
	renderregion "haruki-cloud/internal/pjsk/region"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type frameClientStub struct {
	calls       int
	region, uid string
	frame       *sekaiapi.SponsorFrame
	err         error
}

func (s *frameClientStub) GetSponsorFrameContext(_ context.Context, region, uid string) (*sekaiapi.SponsorFrame, error) {
	s.calls++
	s.region = region
	s.uid = uid
	return s.frame, s.err
}

func validSponsorFrame() *sekaiapi.SponsorFrame {
	parts := func(cell string) sekaiapi.SponsorFrameParts {
		prefix := "static_images/custom_frames/example/" + cell + "/"
		return sekaiapi.SponsorFrameParts{Base: prefix + "base.png", CenterTop: prefix + "ct.png", LeftTop: prefix + "lt.png", RightTop: prefix + "rt.png", LeftBottom: prefix + "lb.png", RightBottom: prefix + "rb.png"}
	}
	return &sekaiapi.SponsorFrame{ID: "example", Horizontal: parts("horizontal"), Vertical: parts("vertical")}
}

func TestPreferencesUseResolvedAccountAndReturnIndependentCopies(t *testing.T) {
	s := &frameClientStub{frame: validSponsorFrame()}
	p := NewToolboxPreferences(s)
	first := p.Lookup(t.Context(), renderregion.CN, "1234")
	if first == nil || first.Horizontal == nil || first.Vertical == nil || s.region != "cn" || s.uid != "1234" {
		t.Fatalf("wrong frame/account: %+v %+v", first, s)
	}
	first.Horizontal.Base = "mutated"
	first.Vertical.Base = "mutated"
	s.frame.Horizontal.Base = "upstream mutated"
	next := p.Lookup(t.Context(), renderregion.CN, "1234")
	if s.calls != 1 || !strings.HasSuffix(next.Horizontal.Base, "horizontal/base.png") || !strings.HasSuffix(next.Vertical.Base, "vertical/base.png") {
		t.Fatal("cached selection was mutated or not reused")
	}
	p.Lookup(t.Context(), renderregion.JP, "1234")
	if s.calls != 2 {
		t.Fatal("region not included in cache key")
	}
	// A removal/unlink takes effect after the short cache lifetime.
	p.mu.Lock()
	p.cache["cn:1234"] = preferenceEntry{frame: validSponsorFrame(), expires: time.Now().Add(-time.Second)}
	p.mu.Unlock()
	s.frame = nil
	if p.Lookup(t.Context(), renderregion.CN, "1234") != nil || s.calls != 3 {
		t.Fatal("expired decoration survived unlink")
	}
}

func TestPreferencesRejectUntrustedPathsAndFallBackOnOutage(t *testing.T) {
	for _, bad := range []string{"https://evil.example/frame.png", "/etc/passwd", "static_images/custom_frames/../secret.png", "static_images/custom_frames/a\\b.png", "static_images/not-a-frame.png"} {
		frame := validSponsorFrame()
		frame.Vertical.LeftTop = bad
		if preferencePaths(frame) != nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	s := &frameClientStub{err: errors.New("unavailable")}
	p := NewToolboxPreferences(s)
	o := NewOverrides([]config.PlayerFrameOverride{{Server: "cn", UserID: "1234", Horizontal: config.PlayerFrameParts{Base: "existing-operator-frame.png"}}})
	got := ResolvePreferredAccount(t.Context(), p, nil, o, renderregion.CN, "1234", nil)
	if got == nil || got.Base != "existing-operator-frame.png" {
		t.Fatal("provider outage broke existing frame")
	}
	if got := ResolvePreferredAccount(t.Context(), p, nil, nil, renderregion.CN, "5678", nil); got != nil {
		t.Fatal("unrelated account inherited frame")
	}
	s.frame = validSponsorFrame()
	s.err = nil
	p = NewToolboxPreferences(s)
	got = ResolvePreferredAccount(t.Context(), p, nil, o, renderregion.CN, "1234", nil)
	if got == nil || got.Base != s.frame.Horizontal.Base {
		t.Fatal("saved user choice did not override configured default")
	}
}
