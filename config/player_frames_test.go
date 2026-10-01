package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func frameEntry() PlayerFrameOverride {
	p := PlayerFrameParts{Base: "static_images/a/base.png", CenterTop: "static_images/b/center.png", LeftTop: "static_images/c/lt.png", RightTop: "static_images/d/rt.png", LeftBottom: "static_images/e/lb.png", RightBottom: "static_images/f/rb.png"}
	return PlayerFrameOverride{Server: "cn", UserID: "1234567890123456789", Horizontal: p}
}

func TestValidatePlayerFrameOverrides(t *testing.T) {
	if err := ValidatePlayerFrameOverrides([]PlayerFrameOverride{frameEntry()}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*PlayerFrameOverride)
	}{
		{"server", func(e *PlayerFrameOverride) { e.Server = "CN" }},
		{"uid", func(e *PlayerFrameOverride) { e.UserID = "01" }},
		{"empty part", func(e *PlayerFrameOverride) { e.Horizontal.RightTop = "" }},
		{"traversal", func(e *PlayerFrameOverride) { e.Horizontal.Base = "static_images/../secret" }},
		{"remote", func(e *PlayerFrameOverride) { e.Horizontal.Base = "https://example.com/a.png" }},
		{"absolute", func(e *PlayerFrameOverride) { e.Horizontal.Base = "/tmp/a.png" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := frameEntry()
			tc.edit(&e)
			if ValidatePlayerFrameOverrides([]PlayerFrameOverride{e}) == nil {
				t.Fatal("invalid entry accepted")
			}
		})
	}
	if ValidatePlayerFrameOverrides([]PlayerFrameOverride{frameEntry(), frameEntry()}) == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestValidatePlayerFrameOverridesOptionalVertical(t *testing.T) {
	withVertical := func(edit func(*PlayerFrameParts)) PlayerFrameOverride {
		e := frameEntry()
		v := e.Horizontal
		v.Base = "static_images/v/base.png"
		if edit != nil {
			edit(&v)
		}
		e.Vertical = &v
		return e
	}
	if err := ValidatePlayerFrameOverrides([]PlayerFrameOverride{withVertical(nil)}); err != nil {
		t.Fatalf("valid vertical block rejected: %v", err)
	}
	for name, edit := range map[string]func(*PlayerFrameParts){
		"empty part": func(p *PlayerFrameParts) { p.CenterTop = "" },
		"traversal":  func(p *PlayerFrameParts) { p.LeftBottom = "static_images/../x.png" },
		"remote":     func(p *PlayerFrameParts) { p.RightTop = "https://example.com/a.png" },
	} {
		err := ValidatePlayerFrameOverrides([]PlayerFrameOverride{withVertical(edit)})
		if err == nil || !strings.Contains(err.Error(), ".vertical.") {
			t.Fatalf("%s: invalid vertical part not reported as vertical: %v", name, err)
		}
	}
}

func TestPlayerFrameOverridesReadOnlyFromYAML(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_PLAYER_FRAME_OVERRIDES", `[{"server":"jp"}]`)
	p := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(p, []byte(`pjsk_render:
  player_frame_overrides:
    - server: cn
      user_id: "1234567890123456789"
      horizontal:
        base: static_images/a/base.png
        centertop: static_images/b/center.png
        lefttop: static_images/c/lt.png
        righttop: static_images/d/rt.png
        leftbottom: static_images/e/lb.png
        rightbottom: static_images/f/rb.png
`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.PJSKRender.PlayerFrameOverrides) != 1 || c.PJSKRender.PlayerFrameOverrides[0] != frameEntry() {
		t.Fatal("YAML configuration was not retained")
	}
}

func TestPlayerFrameOverridesReadVerticalFromYAML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	err := os.WriteFile(p, []byte(`pjsk_render:
  player_frame_overrides:
    - server: cn
      user_id: "1234567890123456789"
      horizontal:
        base: static_images/a/base.png
        centertop: static_images/b/center.png
        lefttop: static_images/c/lt.png
        righttop: static_images/d/rt.png
        leftbottom: static_images/e/lb.png
        rightbottom: static_images/f/rb.png
      vertical:
        base: static_images/v/base.png
        centertop: static_images/v/center.png
        lefttop: static_images/v/lt.png
        righttop: static_images/v/rt.png
        leftbottom: static_images/v/lb.png
        rightbottom: static_images/v/rb.png
`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	got := c.PJSKRender.PlayerFrameOverrides
	if len(got) != 1 || got[0].Vertical == nil || got[0].Vertical.Base != "static_images/v/base.png" || got[0].Vertical.RightBottom != "static_images/v/rb.png" {
		t.Fatalf("vertical block not read: %+v", got)
	}
}
