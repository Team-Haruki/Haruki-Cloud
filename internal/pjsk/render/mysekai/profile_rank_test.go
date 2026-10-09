package mysekai

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
)

func TestMysekaiProfileCardKeepsSuiteRank(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{AllowFallback: true})
	rank := 321
	override := &drawing.ProfileCardRequest{
		Profile:     &drawing.BasicProfile{ID: "1", Region: "JP"},
		DataSources: []drawing.ProfileDataSource{{Name: common.DataSourceLabel(drawing.DataSourceSuite), Kind: drawing.DataSourceSuite}},
		Rank:        &rank,
	}
	merged := map[string]any{"userMysekaiGamedata": map[string]any{"mysekaiRank": 10}}

	for _, includeSuite := range []bool{true, false} {
		got := controller.mysekaiProfileCard(renderregion.JP, merged, override, includeSuite)
		if got.Rank == nil || *got.Rank != rank || got.Rank == override.Rank {
			t.Fatalf("includeSuite=%v rank = %v, want a copy of %d", includeSuite, got.Rank, rank)
		}
		if got.MysekaiLevel == nil || *got.MysekaiLevel != 10 {
			t.Fatalf("includeSuite=%v mysekai level = %v", includeSuite, got.MysekaiLevel)
		}
	}
}
