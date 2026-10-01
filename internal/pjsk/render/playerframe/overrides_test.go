package playerframe

import (
	"testing"

	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type overrideTestSource struct{ region renderregion.Value }

func (s overrideTestSource) DefaultRegion() renderregion.Value { return s.region }
func (overrideTestSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	return &masterdata.PlayerFrame{ID: id, PlayerFrameGroupID: 1}, nil
}
func (overrideTestSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return &masterdata.PlayerFrameGroup{ID: id, AssetBundleName: "frame_0001", PlayerFrameType: "single"}, nil
}
func (overrideTestSource) GetPlayerFramePartsByGroupID(int) (map[int]int, error) { return nil, nil }

func TestResolveAccountPrefersOverrideAndIsolatesCopies(t *testing.T) {
	parts := config.PlayerFrameParts{Base: "static_images/b.png", CenterTop: "static_images/ct.png", LeftTop: "static_images/lt.png", RightTop: "static_images/rt.png", LeftBottom: "static_images/lb.png", RightBottom: "static_images/rb.png"}
	entries := []config.PlayerFrameOverride{{Server: "cn", UserID: "42", Horizontal: parts}}
	o := NewOverrides(entries)
	entries[0].Horizontal.Base = "mutated"
	equipped := []UserFrame{{PlayerFrameID: 10001, PlayerFrameAttachStatus: "first"}}
	src := overrideTestSource{region: renderregion.CN}

	got := ResolveAccount(src, o, renderregion.Normalize("CN"), " 42 ", equipped)
	if got == nil || got.Base != parts.Base || got.Horizontal == nil || *got.Horizontal != (drawing.PlayerFrameParts{Base: parts.Base, CenterTop: parts.CenterTop, LeftTop: parts.LeftTop, RightTop: parts.RightTop, LeftBottom: parts.LeftBottom, RightBottom: parts.RightBottom}) {
		t.Fatalf("override not applied: %+v", got)
	}
	got.Horizontal.Base = "request mutation"
	if again, _ := o.Lookup(renderregion.CN, "42"); again.Horizontal.Base != parts.Base {
		t.Fatal("request mutated the shared override")
	}
	if game := ResolveAccount(src, o, renderregion.JP, "42", equipped); game == nil || game.Horizontal != nil || game.FrameType != "single" {
		t.Fatalf("override leaked to another server: %+v", game)
	}
	if none := ResolveAccount(nil, o, renderregion.CN, "43", equipped); none != nil {
		t.Fatalf("expected no frame without a source: %+v", none)
	}
	if fromOverrideOnly := ResolveAccountFromProvider(t.Context(), nil, o, renderregion.CN, "42", nil); fromOverrideOnly == nil {
		t.Fatal("override must apply even without masterdata")
	}
}

func TestOverrideVerticalPartsAreOptionalAndIsolated(t *testing.T) {
	h := config.PlayerFrameParts{Base: "static_images/h/b.png", CenterTop: "static_images/h/ct.png", LeftTop: "static_images/h/lt.png", RightTop: "static_images/h/rt.png", LeftBottom: "static_images/h/lb.png", RightBottom: "static_images/h/rb.png"}
	v := config.PlayerFrameParts{Base: "static_images/v/b.png", CenterTop: "static_images/v/ct.png", LeftTop: "static_images/v/lt.png", RightTop: "static_images/v/rt.png", LeftBottom: "static_images/v/lb.png", RightBottom: "static_images/v/rb.png"}
	entries := []config.PlayerFrameOverride{
		{Server: "jp", UserID: "1", Horizontal: h, Vertical: &v},
		{Server: "jp", UserID: "2", Horizontal: h},
	}
	o := NewOverrides(entries)
	entries[0].Vertical.Base = "mutated"

	both, ok := o.Lookup(renderregion.JP, "1")
	if !ok || both.Horizontal == nil || both.Vertical == nil {
		t.Fatalf("expected both cells: %+v", both)
	}
	if both.Vertical.Base != "static_images/v/b.png" || both.Vertical.RightBottom != v.RightBottom || both.Horizontal.Base != h.Base {
		t.Fatalf("cells mixed up or config mutation leaked: %+v %+v", both.Horizontal, both.Vertical)
	}
	// the top-level paths stay the horizontal ones (what older Drawing builds read)
	if both.Base != h.Base {
		t.Fatalf("top-level base = %q, want the horizontal sprite", both.Base)
	}
	both.Vertical.Base = "request mutation"
	if again, _ := o.Lookup(renderregion.JP, "1"); again.Vertical.Base != "static_images/v/b.png" {
		t.Fatal("request mutated the shared vertical override")
	}
	if only, _ := o.Lookup(renderregion.JP, "2"); only == nil || only.Horizontal == nil || only.Vertical != nil {
		t.Fatalf("horizontal-only override must not invent vertical parts: %+v", only)
	}
}
