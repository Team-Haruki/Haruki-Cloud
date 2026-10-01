package config

import (
	"os"
	"path/filepath"
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
