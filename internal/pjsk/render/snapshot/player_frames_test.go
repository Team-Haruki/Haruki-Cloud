package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

func TestSnapshotFrameEnrichmentPreservesCacheAndCardConversions(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"playerFrames.json":      `[{"id":20001,"playerFrameGroupId":2}]`,
		"playerFrameGroups.json": `[{"id":2,"assetbundleName":"frame_0002","playerFrameType":"combination"}]`,
		"playerFrameParts.json":  `[{"id":20021,"playerFrameGroupId":2,"gameCharacterId":21}]`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := NewDefaultSnapshotFactory(nil, nil).Build(t.Context(), BuildInput{Region: renderregion.EN, SuiteJSON: []byte(`{
 "userGamedata":{"userId":123,"name":"frame test"},
 "userPlayerFrames":[{"playerFrameId":20001,"playerFrameAttachStatus":"first","partsLayout":[
 {"partPosition":"part1","gameCharacterId":21},{"partPosition":"part2","gameCharacterId":21},
 {"partPosition":"part3","gameCharacterId":21},{"partPosition":"part4","gameCharacterId":21},
 {"partPosition":"part5","gameCharacterId":21},{"partPosition":"part6","gameCharacterId":21}]}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	original := snap.(*Service)
	enriched := original.WithPlayerFrames(t.Context(), provider.NewLocalProvider(root, renderregion.EN).PlayerFrames())
	detail, card := enriched.DetailedProfile(renderregion.EN), enriched.ProfileCard(renderregion.EN)
	if !detail.HasFrame || detail.FramePaths == nil || card.Profile.FramePaths == nil || card.Profile.FramePaths.SideLeftTop != "asset/en-assets/startapp/player_frame/frame_0002/20001/20021/vertical/frame_parts2_left.png" {
		t.Fatalf("lost frame in card conversion: %+v", card.Profile)
	}
	if enriched.baseProfile.Region != "EN" || original.baseProfile.HasFrame || original.baseProfile.FramePaths != nil {
		t.Fatal("enrichment mutated the cached snapshot or region")
	}
	detail.FramePaths.Base = "mutated"
	card.Profile.FramePaths.LeftTop = "mutated"
	again := enriched.DetailedProfile(renderregion.EN)
	if again.FramePaths.Base == "mutated" || again.FramePaths.LeftTop == "mutated" {
		t.Fatal("frame paths escaped as a mutable shared pointer")
	}
}
