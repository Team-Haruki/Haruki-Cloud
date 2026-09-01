package music

import (
	"reflect"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestResolveMusicBoardSkillsStrategies(t *testing.T) {
	if got := resolveMusicBoardSkills(musicBoardResolvedQuery{SkillStrategy: "max", Skills: []float64{1, 3, 2}}); !reflect.DeepEqual(got, []float64{3, 2, 1}) {
		t.Fatalf("max skills = %#v", got)
	}
	if got := resolveMusicBoardSkills(musicBoardResolvedQuery{SkillStrategy: "min", Skills: []float64{1, 3, 2}}); !reflect.DeepEqual(got, []float64{1, 2, 3}) {
		t.Fatalf("min skills = %#v", got)
	}
	if got := resolveMusicBoardSkills(musicBoardResolvedQuery{SkillStrategy: "avg", Skills: []float64{1, 2, 3}}); !reflect.DeepEqual(got, []float64{2, 2, 2}) {
		t.Fatalf("average skills = %#v", got)
	}
	if got := resolveMusicBoardSkills(musicBoardResolvedQuery{SkillStrategy: "avg"}); len(got) != 0 {
		t.Fatalf("empty average skills = %#v", got)
	}
}

func TestBuildMusicBoardMetaRowBranches(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{1: {ID: 1, Title: "Board Song", AssetBundleName: "board_song"}},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31}},
		},
	}
	builder := NewBuilder(source, nil, nil)
	query := musicBoardResolvedQuery{Skills: []float64{1, 1, 1, 1, 1}, Power: 100, DeckBonus: 10, PlayInterval: 5}
	meta := drawing.MusicMetaInfo{Difficulty: "master", EventRate: 100, BaseScore: 1, BaseScoreAuto: 1}
	row, ok := buildMusicBoardMetaRow(builder, renderregion.JP, source.musics[1], 1, meta, query, query.Skills)
	if !ok || row.Level != 31 || row.Tps != 0 || row.SoloScore == nil || row.MultiScore == nil {
		t.Fatalf("built row = %+v, %v", row, ok)
	}
	meta.Difficulty = "append"
	if _, ok := buildMusicBoardMetaRow(builder, renderregion.JP, source.musics[1], 1, meta, query, query.Skills); ok {
		t.Fatal("missing difficulty unexpectedly built a row")
	}
	if got := rankedMusicBoardRows([]musicBoardRow{{Rank: 0}, {Rank: 2}}); len(got) != 1 || got[0].Rank != 2 {
		t.Fatalf("ranked rows = %#v", got)
	}
}

func TestMusicBoardSpecHelpers(t *testing.T) {
	rows := []musicBoardRow{
		{Rank: 1, MusicID: 1, Difficulty: "expert"},
		{Rank: 2, MusicID: 1, Difficulty: "master"},
		{Rank: 3, MusicID: 2, Difficulty: "hard"},
	}
	available := availableMusicBoardDifficulties(rows)
	if !reflect.DeepEqual(available[1], []string{"expert", "master"}) {
		t.Fatalf("available difficulties = %#v", available)
	}
	if matches, err := resolveMusicBoardSpec(nil, rows, available, " "); err != nil || matches != nil {
		t.Fatalf("empty spec = %#v, %v", matches, err)
	}
	if _, err := resolveMusicBoardSpec(nil, rows, available, "*"); err == nil {
		t.Fatal("empty wildcard spec unexpectedly succeeded")
	}
	matches := allMusicBoardSpecsForMusic(rows, 1)
	if len(matches) != 2 || matches[0].Difficulty != "master" {
		t.Fatalf("sorted specs = %#v", matches)
	}
	seen := make(map[string]struct{})
	got := appendUniqueMusicBoardSpecs(nil, seen, append(matches, matches[0]))
	if len(got) != 2 {
		t.Fatalf("unique specs = %#v", got)
	}
}

func TestMusicBoardSelectionAndPaginationHelpers(t *testing.T) {
	rows := make([]musicBoardRow, 60)
	for index := range rows {
		rows[index] = musicBoardRow{Rank: index + 1, MusicID: index + 1, Difficulty: "master", Level: 30}
	}
	specRows, excluded := selectMusicBoardSpecRows(rows, []musicBoardSpec{{MusicID: 1, Difficulty: "master"}, {MusicID: 999, Difficulty: "master"}})
	if len(specRows) != 1 || len(excluded) != 1 {
		t.Fatalf("selected specs = %#v, %#v", specRows, excluded)
	}
	filtered := filterMusicBoardRows(rows, excluded, musicBoardResolvedQuery{DiffFilter: []string{"master"}, LevelFilter: "30"})
	if len(filtered) != 59 {
		t.Fatalf("filtered rows = %d", len(filtered))
	}
	page, total, err := paginateMusicBoardRows(specRows, filtered, 2)
	if err != nil || total != 2 || len(page) != 11 || page[0].Rank != 1 {
		t.Fatalf("page = %#v, total=%d, err=%v", page, total, err)
	}
	if _, _, err := paginateMusicBoardRows(nil, filtered, 3); err == nil {
		t.Fatal("out-of-range page unexpectedly succeeded")
	}
	if _, _, err := paginateMusicBoardRows(nil, nil, 1); err == nil {
		t.Fatal("empty page unexpectedly succeeded")
	}
}

func TestMusicBoardDrawingConversionHelpers(t *testing.T) {
	value := 12.0
	rows := []musicBoardRow{{
		Rank: 1, MusicID: 7, Difficulty: "master", Level: 31, MusicTitle: "Song",
		SoloPt: &value, SoloRealScore: &value, SoloScore: &value, SoloSkillAccount: &value,
		SoloPtPerHour: &value, PlayCountPerHour: &value, EventRate: 100, MusicTime: 120, Tps: 5,
	}}
	items := musicBoardDrawingItems(rows, "solo")
	if len(items) != 1 || items[0].MusicID != 7 || items[0].LiveTypeScore == nil {
		t.Fatalf("drawing items = %#v", items)
	}
	midDiffs := musicBoardSpecMidDiffs([]musicBoardSpec{{MusicID: 7, Difficulty: "master"}})
	if len(midDiffs) != 1 || midDiffs[0][0] != 7 {
		t.Fatalf("spec tuples = %#v", midDiffs)
	}
}
