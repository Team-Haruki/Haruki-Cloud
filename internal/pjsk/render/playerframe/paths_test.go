package playerframe

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

func TestEquippedFramePaths(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"playerFrames.json":      `[{"id":10001,"playerFrameGroupId":1},{"id":20001,"playerFrameGroupId":2}]`,
		"playerFrameGroups.json": `[{"id":1,"assetbundleName":"frame_0001","playerFrameType":"single"},{"id":2,"assetbundleName":"frame_0002","playerFrameType":"combination"}]`,
		// Deliberately non-arithmetic IDs: the mapping must come from masterdata.
		"playerFrameParts.json": `[{"id":301,"playerFrameGroupId":2,"gameCharacterId":1},{"id":407,"playerFrameGroupId":2,"gameCharacterId":2},{"id":999,"playerFrameGroupId":1,"gameCharacterId":1}]`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := provider.NewLocalProvider(root, renderregion.JP).PlayerFrames()
	single := Resolve(t.Context(), p, renderregion.JP, []UserFrame{{PlayerFrameID: 10001, PlayerFrameAttachStatus: "first"}})
	if single == nil || single.FrameType != "single" || single.Base != "asset/jp-assets/startapp/player_frame/frame_0001/10001/vertical/frame_base.png" || single.LeftTop != "asset/jp-assets/startapp/player_frame/frame_0001/10001/vertical/frame_lefttop.png" {
		t.Fatalf("single: %+v", single)
	}
	layout := []PartLayout{{"part6", 2}, {"part3", 1}, {"part1", 2}, {"part4", 1}, {"part5", 2}, {"part2", 1}}
	frames := []UserFrame{{PlayerFrameID: 10001, PlayerFrameAttachStatus: "none"}, {PlayerFrameID: 20001, PlayerFrameAttachStatus: "first", PartsLayout: layout}}
	combo := Resolve(t.Context(), p, renderregion.JP, frames)
	if combo == nil || combo.FrameType != "combination" || combo.Base != "asset/jp-assets/startapp/player_frame/frame_0002/20001/407/vertical/frame_base.png" || combo.SideLeftTop != "asset/jp-assets/startapp/player_frame/frame_0002/20001/301/vertical/frame_parts2_left.png" || combo.LeftBottom != "asset/jp-assets/startapp/player_frame/frame_0002/20001/407/vertical/frame_parts6_left.png" {
		t.Fatalf("combination: %+v", combo)
	}
	for _, invalid := range [][]PartLayout{nil, layout[:5], append(append([]PartLayout{}, layout...), layout[0]), {{"part1", 99}, {"part2", 1}, {"part3", 1}, {"part4", 1}, {"part5", 1}, {"part6", 1}}} {
		frames[1].PartsLayout = invalid
		if got := Resolve(t.Context(), p, renderregion.JP, frames); got != nil {
			t.Fatalf("invalid layout produced guessed art: %+v", got)
		}
	}
	if got := Resolve(t.Context(), p, renderregion.JP, frames[:1]); got != nil {
		t.Fatal("unequipped frame was rendered")
	}
	if got := Resolve(t.Context(), p, renderregion.JP, []UserFrame{{PlayerFrameID: 10001}}); got != nil {
		t.Fatal("a frame without an attach status was rendered")
	}
}
