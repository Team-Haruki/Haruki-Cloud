package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/config"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/playerframe"
	"haruki-cloud/internal/pjsk/render/provider"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func writePlayerFrameMaster(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{
		"playerFrames.json":      `[{"id":10001,"playerFrameGroupId":1}]`,
		"playerFrameGroups.json": `[{"id":1,"assetbundleName":"frame_0001","playerFrameType":"single"}]`,
		"playerFrameParts.json":  `[]`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func buildFrameSnapshot(t *testing.T, region renderregion.Value, uid string, equipped bool) rendersnapshot.Snapshot {
	t.Helper()
	frames := `[]`
	if equipped {
		frames = `[{"playerFrameId":10001,"playerFrameAttachStatus":"first"}]`
	}
	snap, err := rendersnapshot.NewDefaultSnapshotFactory(nil, nil).Build(t.Context(), rendersnapshot.BuildInput{
		Region:    region,
		SuiteJSON: []byte(`{"userGamedata":{"userId":` + uid + `,"name":"frame test"},"userPlayerFrames":` + frames + `}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// Every info panel built from the suite snapshot (card box/detail, deck, education,
// event, inventory, mysekai, /信息面板 ...) resolves frames in resolveSnapshotBySelectorWithError.
// It must agree with the profile controller: the equipped game frame on every server, and the
// file-configured override for the configured account.
func TestSnapshotInfoPanelFramesMatchProfileResolver(t *testing.T) {
	root := writePlayerFrameMaster(t)
	override := config.PlayerFrameParts{
		Base: "static_images/custom/base.png", CenterTop: "static_images/custom/ct.png",
		LeftTop: "static_images/custom/lt.png", RightTop: "static_images/custom/rt.png",
		LeftBottom: "static_images/custom/lb.png", RightBottom: "static_images/custom/rb.png",
	}
	app := &renderapp.App{
		Providers: map[renderregion.Value]provider.MasterDataProvider{
			renderregion.JP: provider.NewLocalProvider(root, renderregion.JP),
			renderregion.CN: provider.NewLocalProvider(root, renderregion.CN),
		},
		FrameOverrides: playerframe.NewOverrides([]config.PlayerFrameOverride{{Server: "cn", UserID: "7777", Horizontal: override}}),
	}

	cases := []struct {
		name     string
		region   renderregion.Value
		uid      string
		equipped bool
		wantBase string
		wantH    bool
	}{
		{"jp equipped game frame", renderregion.JP, "1111", true, "asset/jp-assets/startapp/player_frame/frame_0001/10001/vertical/frame_base.png", false},
		{"cn equipped game frame", renderregion.CN, "2222", true, "asset/cn-assets/startapp/player_frame/frame_0001/10001/vertical/frame_base.png", false},
		{"cn file override", renderregion.CN, "7777", true, override.Base, true},
		{"cn file override without a game frame", renderregion.CN, "7777", false, override.Base, true},
		{"jp account with the overridden uid keeps its own frame", renderregion.JP, "7777", true, "asset/jp-assets/startapp/player_frame/frame_0001/10001/vertical/frame_base.png", false},
		{"no frame", renderregion.JP, "3333", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &runtimeSnapshotProviderStub{snapshot: buildFrameSnapshot(t, tc.region, tc.uid, tc.equipped)}
			original := snapshotProviderFactory
			snapshotProviderFactory = func(*renderapp.App) rendersnapshot.HarukiSnapshotProvider { return stub }
			t.Cleanup(func() { snapshotProviderFactory = original })

			snap, err := resolveSnapshotBySelectorWithError(context.Background(), app, rendersnapshot.Selector{Region: tc.region}, rendersnapshot.ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			detail, card := snap.DetailedProfile(tc.region), snap.ProfileCard(tc.region)
			if tc.wantBase == "" {
				if detail.HasFrame || detail.FramePaths != nil || card.Profile.FramePaths != nil {
					t.Fatalf("unexpected frame: %+v", detail.FramePaths)
				}
				return
			}
			if !detail.HasFrame || detail.FramePaths == nil || detail.FramePaths.Base != tc.wantBase {
				t.Fatalf("detailed profile frame = %+v, want base %q", detail.FramePaths, tc.wantBase)
			}
			if !card.Profile.HasFrame || card.Profile.FramePaths == nil || card.Profile.FramePaths.Base != tc.wantBase {
				t.Fatalf("profile card frame = %+v, want base %q", card.Profile.FramePaths, tc.wantBase)
			}
			if (detail.FramePaths.Horizontal != nil) != tc.wantH {
				t.Fatalf("horizontal override parts present = %v, want %v", detail.FramePaths.Horizontal != nil, tc.wantH)
			}

			// Same inputs through the profile controller's resolver give the same paths.
			var frames []playerframe.UserFrame
			if tc.equipped {
				frames = []playerframe.UserFrame{{PlayerFrameID: 10001, PlayerFrameAttachStatus: "first"}}
			}
			direct := playerframe.ResolveAccountFromProvider(t.Context(), app.Providers[tc.region].PlayerFrames(), app.FrameOverrides, tc.region, tc.uid, frames)
			if direct == nil || direct.Base != detail.FramePaths.Base {
				t.Fatalf("snapshot path %q diverges from profile resolver %+v", detail.FramePaths.Base, direct)
			}
		})
	}
}
