package deck

import (
	"os"
	"path/filepath"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func TestWL3FinaleMaxProfileIncludesChapterTop1000Honors(t *testing.T) {
	dir := t.TempDir()
	fixtures := map[string]string{
		"events":                               `[{"id":202,"eventType":"world_bloom","assetbundleName":"event_wl_3rd_part1_2026"},{"id":224,"eventType":"world_bloom","assetbundleName":"event_wl_3rd_finale_2026"},{"id":171,"eventType":"world_bloom","assetbundleName":"event_wl_2nd_2025"}]`,
		"worldBlooms":                          `[{"id":1,"eventId":202,"gameCharacterId":1,"chapterNo":1,"worldBloomChapterType":"game_character"},{"id":2,"eventId":202,"gameCharacterId":2,"chapterNo":2,"worldBloomChapterType":"game_character"},{"id":3,"eventId":202,"gameCharacterId":3,"chapterNo":3,"worldBloomChapterType":"game_character"},{"id":4,"eventId":171,"gameCharacterId":1,"chapterNo":1}]`,
		"worldBloomChapterRankingRewardRanges": `[{"eventId":202,"gameCharacterId":1,"fromRank":501,"toRank":1000,"resourceBoxId":11},{"eventId":202,"gameCharacterId":2,"fromRank":501,"toRank":1000,"resourceBoxId":12},{"eventId":202,"gameCharacterId":3,"fromRank":1001,"toRank":2000,"resourceBoxId":13},{"eventId":171,"gameCharacterId":1,"fromRank":501,"toRank":1000,"resourceBoxId":14}]`,
		"resourceBoxes":                        `[{"id":11,"resourceBoxPurpose":"world_bloom_chapter_ranking_reward","details":[{"resourceType":"honor","resourceId":7748}]},{"id":12,"resourceBoxPurpose":"world_bloom_chapter_ranking_reward","details":[{"resourceType":"honor","resourceId":7749}]},{"id":13,"resourceBoxPurpose":"world_bloom_chapter_ranking_reward","details":[{"resourceType":"honor","resourceId":7750}]},{"id":14,"resourceBoxPurpose":"world_bloom_chapter_ranking_reward","details":[{"resourceType":"honor","resourceId":7000}]}]`,
	}
	for name, value := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := renderevent.NewProviderAdapter(provider.NewLocalProvider(dir, renderregion.JP))
	controller := NewController(nil, source, nil, nil, nil, renderregion.JP)
	for _, tc := range []struct {
		name  string
		query AutoQuery
		want  int
	}{
		{"simulated", AutoQuery{MaxProfile: true, WorldBloomFinaleTurn: new(3)}, 3},
		{"released", AutoQuery{MaxProfile: true, EventID: new(224), MetadataWorldBloomFinale: true}, 3},
		{"ordinary chapter", AutoQuery{MaxProfile: true, EventID: new(202)}, 1},
		{"explicit chapter overrides simulation", AutoQuery{MaxProfile: true, EventID: new(202), WorldBloomFinaleTurn: new(3)}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &snapshot.RawUserData{UserHonors: []snapshot.RawUserHonor{{HonorID: 99}}}
			original := &snapshot.RawUserData{UserHonors: []snapshot.RawUserHonor{{HonorID: 99}}}
			controller.applyMaxProfileEventHonors(renderregion.JP, raw, tc.query)
			controller.applyMaxProfileEventHonors(renderregion.JP, raw, tc.query)
			if len(raw.UserHonors) != tc.want {
				t.Fatalf("honors: %+v", raw.UserHonors)
			}
			if tc.want == 3 && (raw.UserHonors[1].HonorID != 7748 || raw.UserHonors[2].HonorID != 7749) {
				t.Fatalf("wrong chapter rewards: %+v", raw.UserHonors)
			}
			encoded, err := encodePreparedRecommendUserData([]byte(`{"userHonors":[{"honorId":99}]}`), original, raw)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				UserHonors []snapshot.RawUserHonor `json:"userHonors"`
			}
			if err = json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.UserHonors) != tc.want {
				t.Fatalf("honors missing from service payload: %s", encoded)
			}
		})
	}
}
