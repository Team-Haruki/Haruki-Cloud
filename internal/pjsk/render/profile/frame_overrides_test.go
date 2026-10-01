package profile

import (
	"haruki-cloud/config"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/sekai"
	"testing"
)

func TestFrameOverridesAreAccountScopedAndImmutable(t *testing.T) {
	p := config.PlayerFrameParts{Base: "static_images/a.png", CenterTop: "static_images/b.png", LeftTop: "static_images/c.png", RightTop: "static_images/d.png", LeftBottom: "static_images/e.png", RightBottom: "static_images/f.png"}
	entries := []config.PlayerFrameOverride{{Server: "cn", UserID: "1234567890123456789", Horizontal: p}}
	c := NewController(nil, nil, nil, nil)
	c.SetFrameOverrides(entries)
	entries[0].Horizontal.Base = "mutated"
	got, ok := c.buildAccountFramePaths(nil, nil, "cn", "1234567890123456789")
	if !ok || got.Horizontal == nil || got.Horizontal.Base != p.Base || got.Horizontal.RightBottom != p.RightBottom {
		t.Fatal("configured independent parts not applied")
	}
	got.Horizontal.Base = "request mutation"
	clone := c.WithContext(t.Context())
	again, _ := clone.buildAccountFramePaths(nil, nil, "cn", "1234567890123456789")
	if again.Horizontal.Base != p.Base {
		t.Fatal("request mutated shared configuration")
	}
	for _, key := range [][2]string{{"jp", "1234567890123456789"}, {"cn", "123"}} {
		if _, ok := c.buildAccountFramePaths(nil, nil, key[0], key[1]); ok {
			t.Fatal("override leaked into another account")
		}
	}
}

func TestProfileBuildersUseActualAccountInsteadOfQueryUID(t *testing.T) {
	c := NewController(&testProfileSource{region: renderregion.CN}, nil, nil, nil)
	p := config.PlayerFrameParts{Base: "static_images/base.png", CenterTop: "static_images/ct.png", LeftTop: "static_images/lt.png", RightTop: "static_images/rt.png", LeftBottom: "static_images/lb.png", RightBottom: "static_images/rb.png"}
	c.SetFrameOverrides([]config.PlayerFrameOverride{{Server: "cn", UserID: "12345", Horizontal: p}})
	query := Query{UserID: "99999", Region: "cn", Visible: true}
	resp := &sekai.GetAnotherProfileResponse{User: sekai.AnotherUser{UserID: 12345, Name: "test"}}
	full, err := c.BuildProfileRequestFromAPI(query, resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if full.FramePaths == nil || full.FramePaths.Horizontal == nil || !full.Profile.HasFrame {
		t.Fatal("full profile override missing")
	}
	detail, err := c.BuildDetailedProfileCardFromAPI(query, resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if detail.FramePaths == nil || detail.FramePaths.Horizontal == nil || !detail.HasFrame {
		t.Fatal("card override missing")
	}
	modular, err := c.BuildModularProfileRequestFromAPIWithSnapshot(query, resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if modular.Profile.FramePaths == nil || modular.Profile.FramePaths.Horizontal == nil || !modular.Profile.HasFrame {
		t.Fatal("modular override missing")
	}
	query.UserID = "12345"
	resp.User.UserID = 99999
	other, err := c.BuildDetailedProfileCardFromAPI(query, resp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if other.HasFrame {
		t.Fatal("request UID selected another account's override")
	}
}
