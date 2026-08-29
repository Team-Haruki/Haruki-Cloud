package music

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

type musicAdapterContextKey struct{}

func TestMusicProviderAdapterLocalAndTagBranches(t *testing.T) {
	root := t.TempDir()
	adapter := NewProviderAdapter(provider.NewLocalProvider(root, renderregion.JP))
	ctx := context.WithValue(context.Background(), musicAdapterContextKey{}, "music-adapter")
	contextual := adapter.WithContext(ctx)
	if contextual == nil {
		t.Fatal("WithContext() returned nil")
	}
	contextAdapter := contextual.(*ProviderAdapter)
	if contextAdapter.Context() != ctx {
		t.Fatal("adapter did not retain context")
	}
	if (*ProviderAdapter)(nil).WithContext(ctx) != nil {
		t.Fatal("nil adapter WithContext() returned a source")
	}

	if _, err := contextAdapter.SearchMusic("missing"); err == nil {
		t.Fatal("SearchMusic() error = nil")
	}
	if _, err := contextAdapter.GetMusicByID(1); err == nil {
		t.Fatal("GetMusicByID() error = nil")
	}
	if _, err := contextAdapter.GetMusicByEventID(1); err == nil {
		t.Fatal("GetMusicByEventID() error = nil")
	}
	if got := contextAdapter.GetMusics(); len(got) != 0 {
		t.Fatalf("GetMusics() = %#v", got)
	}
	if got := contextAdapter.GetBanEvents(1); len(got) != 0 {
		t.Fatalf("GetBanEvents() = %#v", got)
	}
	if _, err := contextAdapter.GetMusicLocalizedTitles(1); err == nil {
		t.Fatal("GetMusicLocalizedTitles() error = nil")
	}
	if _, err := contextAdapter.GetMusicDifficulties(1); err == nil {
		t.Fatal("GetMusicDifficulties() error = nil")
	}
	if _, err := contextAdapter.GetMusicVocals(1); err == nil {
		t.Fatal("GetMusicVocals() error = nil")
	}
	if _, err := contextAdapter.GetMusicTags(1); err == nil {
		t.Fatal("GetMusicTags() error = nil")
	}
	if _, err := contextAdapter.GetCharacterByID(1); err == nil {
		t.Fatal("GetCharacterByID() error = nil")
	}
	if _, err := contextAdapter.GetOutsideCharacterByID(1); err == nil {
		t.Fatal("GetOutsideCharacterByID() error = nil")
	}
	if _, err := contextAdapter.GetPrimaryEventByMusicID(1); err == nil {
		t.Fatal("GetPrimaryEventByMusicID() error = nil")
	}
	if got := contextAdapter.GetLimitedTimeMusics(1); len(got) != 0 {
		t.Fatalf("GetLimitedTimeMusics() = %#v", got)
	}

	if got := (*ProviderAdapter)(nil).GetCustomMusicScoreTagNames([]int{1}); got != nil {
		t.Fatalf("nil adapter tag names = %#v", got)
	}
	if got := (&ProviderAdapter{}).GetCustomMusicScoreTagNames([]int{1}); got != nil {
		t.Fatalf("empty adapter tag names = %#v", got)
	}
	if got := adapter.GetCustomMusicScoreTagNames(nil); got != nil {
		t.Fatalf("empty tag IDs = %#v", got)
	}
	if got := adapter.GetCustomMusicScoreTagNames([]int{1}); got != nil {
		t.Fatalf("missing tag file names = %#v", got)
	}

	tagRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(tagRoot, "customMusicScoreTags.json"), []byte(`[
		{"id":1,"name":" Alpha "},
		{"id":2,"name":""},
		{"id":"bad","name":"invalid"},
		{"id":0,"name":"zero"},
		{"id":3,"name":"Gamma"}
	]`), 0o644); err != nil {
		t.Fatalf("write custom tags: %v", err)
	}
	tagAdapter := NewProviderAdapter(provider.NewLocalProvider(tagRoot, renderregion.JP))
	if got, want := tagAdapter.GetCustomMusicScoreTagNames([]int{1, 1, 2, 3, 99}), []string{"Alpha", "Gamma"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tag names = %#v, want %#v", got, want)
	}
}

func TestMusicParseHelpersAllBranches(t *testing.T) {
	if diff, rest := ExtractMusicDifficulty("   "); diff != "" || rest != "" {
		t.Fatalf("empty difficulty = %q, %q", diff, rest)
	}
	if diff, rest := ExtractMusicDifficulty("theme song"); diff != "" || rest != "theme song" {
		t.Fatalf("embedded alias = %q, %q", diff, rest)
	}
	if diff, rest := ExtractMusicDifficulty("红谱  Song   Name"); diff != "expert" || rest != "Song Name" {
		t.Fatalf("CJK difficulty = %q, %q", diff, rest)
	}

	for _, tt := range []struct {
		text  string
		index int
		alias string
		want  bool
	}{
		{text: "abc", index: -1, alias: "a"},
		{text: "abc", index: 0, alias: ""},
		{text: "abc", index: 2, alias: "bcx"},
		{text: "theme", index: 3, alias: "ma"},
		{text: "max", index: 0, alias: "ma"},
		{text: "红谱", index: 0, alias: "红谱", want: true},
		{text: " ma ", index: 1, alias: "ma", want: true},
	} {
		if got := canExtractMusicDifficultyAlias(tt.text, tt.index, tt.alias); got != tt.want {
			t.Errorf("canExtractMusicDifficultyAlias(%q,%d,%q) = %v, want %v", tt.text, tt.index, tt.alias, got, tt.want)
		}
	}
	if !isASCIIAlias("master") || isASCIIAlias("红谱") || !isASCIILetter('Z') || isASCIILetter('1') {
		t.Fatal("ASCII helper classification mismatch")
	}

	if got := SplitMusicQueries("  "); got != nil {
		t.Fatalf("empty split = %#v", got)
	}
	if got, want := SplitMusicQueries(" one / two|\r\n  \nthree "), []string{"one", "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split queries = %#v, want %#v", got, want)
	}

	for _, input := range []string{"song1", "music", "musicx", "music0"} {
		if id, ok := ParseExplicitMusicID(input); ok || id != 0 {
			t.Errorf("ParseExplicitMusicID(%q) = %d, %v", input, id, ok)
		}
	}
	if id, ok := ParseExplicitMusicID(" MUSIC42 "); !ok || id != 42 {
		t.Fatalf("explicit ID = %d, %v", id, ok)
	}
	for _, input := range []string{"", "id", "idx", "id0", "12x"} {
		if id, ok := ParseImplicitMusicID(input); ok || id != 0 {
			t.Errorf("ParseImplicitMusicID(%q) = %d, %v", input, id, ok)
		}
	}
	if id, ok := ParseImplicitMusicID("id12"); !ok || id != 12 {
		t.Fatalf("implicit prefixed ID = %d, %v", id, ok)
	}
	if id, ok := ParseImplicitMusicID("34"); !ok || id != 34 {
		t.Fatalf("implicit numeric ID = %d, %v", id, ok)
	}
	if isNumeric("") || isNumeric("1x") || !isNumeric("123") {
		t.Fatal("numeric classification mismatch")
	}

	parser := NewParser(map[string]int{"ick": 1})
	if _, err := parser.Parse("   "); err == nil {
		t.Fatal("empty parser input error = nil")
	}
	if info, err := parser.Parse("eventoops"); err != nil || info.Type != QueryTypeTitle {
		t.Fatalf("invalid event fallback = %#v, %v", info, err)
	}
	if info := parser.tryParseEvent("event12"); info == nil || info.Value != 12 {
		t.Fatalf("event parse = %#v", info)
	}
	if info := parser.tryParseEvent("eventx"); info != nil {
		t.Fatalf("invalid event parse = %#v", info)
	}
	if info := parser.tryParseSeq("-"); info != nil {
		t.Fatalf("invalid seq parse = %#v", info)
	}
	if info := parser.tryParseBan("ickx"); info != nil {
		t.Fatalf("invalid ban parse = %#v", info)
	}
}

func TestMusicBoardMetricAndFilterBranches(t *testing.T) {
	if got := weightedMusicBoardSkill(nil, nil, 0); got != 0 {
		t.Fatalf("empty weighted skill = %v", got)
	}
	if got := weightedMusicBoardSkill([]float64{1, 2, 3, 4, 5, 6}, []float64{2, 1}, 3); got != 32 {
		t.Fatalf("weighted skill = %v", got)
	}
	if musicBoardSkillAccount(10, 0) != 0 || musicBoardSkillAccount(10, 20) != 0.5 {
		t.Fatal("skill account mismatch")
	}

	populateMusicBoardLiveMetrics(nil, "solo", 1, 1, 1, 1, 1)
	row := musicBoardRow{MusicTime: 100, EventRate: 100}
	populateMusicBoardLiveMetrics(&row, "solo", 1.2, 0.2, 100, 10, 5)
	populateMusicBoardLiveMetrics(&row, "auto", 1.1, 0.3, 100, 10, 5)
	populateMusicBoardLiveMetrics(&row, "multi", 1.3, 0.4, 100, 10, 5)
	populateMusicBoardLiveMetrics(&row, "unknown", 1, 0, 0, 0, -100)
	if row.SoloPt == nil || row.AutoPt == nil || row.MultiPt == nil || row.PlayCountPerHour == nil {
		t.Fatalf("populated metrics = %+v", row)
	}

	rows := []musicBoardRow{
		{MusicID: 1, Difficulty: "expert", Tps: 3, MusicTime: 100},
		{MusicID: 1, Difficulty: "master", Tps: 3, MusicTime: 90},
		{MusicID: 2, Difficulty: "easy", Tps: 1, MusicTime: 120},
	}
	sortMusicBoardRows(rows, "tps", "solo", false, true)
	if rows[0].Difficulty != "master" || rows[0].Rank != 1 || rows[1].Rank != 0 || rows[2].Rank != 2 {
		t.Fatalf("deduplicated ranks = %+v", rows)
	}
	sortMusicBoardRows(rows, "time", "solo", true, false)
	for i := range rows {
		if rows[i].Rank != i+1 {
			t.Fatalf("rank %d = %d", i, rows[i].Rank)
		}
	}

	value := 7.0
	metricRow := musicBoardRow{
		Tps: 8, MusicTime: 9,
		SoloScore: &value, SoloRealScore: &value, SoloPt: &value, SoloPtPerHour: &value, SoloSkillAccount: &value,
		AutoScore: &value, AutoRealScore: &value, AutoPt: &value, AutoPtPerHour: &value, AutoSkillAccount: &value,
		MultiScore: &value, MultiRealScore: &value, MultiPt: &value, MultiPtPerHour: &value, MultiSkillAccount: &value,
	}
	for _, target := range []string{"score", "pt", "pt/time", "tps", "time", "unknown"} {
		_ = musicBoardMetric(metricRow, target, "solo")
	}
	for _, liveType := range []string{"solo", "auto", "multi"} {
		for _, metric := range []string{"score", "real_score", "pt", "pt/time", "pt_per_hour", "skill_account", "unknown"} {
			got := selectMusicBoardLiveValue(metricRow, liveType, metric)
			if metric != "unknown" && got == nil {
				t.Errorf("select value %s/%s = nil", liveType, metric)
			}
		}
	}
	if selectMusicBoardLiveValue(metricRow, "unknown", "score") != nil || derefMusicBoardFloat(nil) != 0 || derefMusicBoardFloat(&value) != value {
		t.Fatal("live value fallback mismatch")
	}

	filters := map[string]bool{
		"": true, "oops": true, "<x": true,
		"<11": true, ">9": true, "<=10": true, ">=10": true, "=10": true, "==10": true,
		"<10": false, ">10": false, "=9": false,
	}
	for filter, want := range filters {
		if got := matchesMusicBoardLevelFilter(10, filter); got != want {
			t.Errorf("level filter %q = %v, want %v", filter, got, want)
		}
	}
}

func TestMusicBoardTextQueryAndSmallHelpers(t *testing.T) {
	for _, difficulty := range []string{"master", "append", "expert", "hard", "normal", "easy", "unknown"} {
		_ = boardDifficultyPriority(difficulty)
		_ = difficultyOrder(difficulty)
	}
	values := []string{"a"}
	if got := appendUniqueString(values, "a"); len(got) != 1 {
		t.Fatalf("existing append = %#v", got)
	}
	if got := appendUniqueString(values, "b"); len(got) != 2 {
		t.Fatalf("new append = %#v", got)
	}
	if minInt(1, 2) != 1 || minInt(2, 1) != 1 || musicBoardKey(3, "MAS") != "3:mas" {
		t.Fatal("small board helper mismatch")
	}

	for _, query := range []musicBoardResolvedQuery{
		{LiveType: "solo", Target: "score", Page: 1, Skills: []float64{1, 1, 1, 1, 1}, SkillStrategy: "max"},
		{LiveType: "multi", Target: "pt", Page: 1, Skills: []float64{1, 1, 1, 1, 1}, Power: 100, DeckBonus: 200},
		{LiveType: "multi", Target: "pt/time", Page: 1, Skills: []float64{1, 1, 1, 1, 1}, Power: 100, DeckBonus: 200, PlayInterval: 5},
		{LiveType: "auto", Target: "time", Ascend: true, Page: 1, Skills: []float64{1, 1, 1, 1, 1}, PlayInterval: 3},
		{LiveType: "solo", Target: "tps", Page: 1, Skills: []float64{1, 1, 1, 1, 1}},
	} {
		title, subtitle := buildMusicBoardTexts(query, 3)
		if !strings.Contains(title, "第1页/共3页") {
			t.Fatalf("board title = %q", title)
		}
		_ = subtitle
	}

	resolved := normalizeMusicBoardQuery(BoardQuery{
		LiveType: "multi", Target: "bad", SkillStrategy: "bad", Page: -1,
		Skills: []float64{0, 1.5}, DiffFilter: []string{"master", "MASTER", ""}, SpecQueries: []string{" ", "song"},
	})
	if resolved.LiveType != "multi" || resolved.Target != "pt/time" || resolved.Page != 1 || len(resolved.Skills) != 5 || len(resolved.DiffFilter) != 1 || len(resolved.SpecQueries) != 1 {
		t.Fatalf("normalized board query = %+v", resolved)
	}
	if got := normalizeMusicBoardQuery(BoardQuery{LiveType: "bad"}); got.LiveType != "solo" || got.Target != "score" || got.SkillStrategy != "max" {
		t.Fatalf("default board query = %+v", got)
	}
	if got := normalizeMusicBoardSkills([]float64{1, 2, 3, 4, 5, 6}, "solo"); !reflect.DeepEqual(got, []float64{1, 2, 3, 4, 5}) {
		t.Fatalf("trimmed skills = %#v", got)
	}
	if got := normalizeMusicBoardSkills(nil, "multi"); len(got) != 5 {
		t.Fatalf("default multi skills = %#v", got)
	}
}

func TestMusicVisibilityAllFallbacks(t *testing.T) {
	now := time.Now().UnixMilli()
	visible := &masterdata.Music{ID: 1, Seq: 10, PublishedAt: now - 1}
	future := &masterdata.Music{ID: 2, PublishedAt: now + 100_000}
	hidden := &masterdata.Music{ID: 241, PublishedAt: now - 1}
	if isMusicAccessibleAt(nil, now, true) || isMusicAccessibleAt(hidden, now, true) || isMusicAccessibleAt(future, now, false) || !isMusicAccessibleAt(future, now, true) || !isMusicVisibleAt(visible, now) {
		t.Fatal("music accessibility mismatch")
	}
	if got, err := ensureAccessibleMusic(visible, now, 1, false); err != nil || got != visible {
		t.Fatalf("accessible music = %#v, %v", got, err)
	}
	for _, tt := range []struct {
		music    *masterdata.Music
		fallback any
	}{
		{music: future, fallback: 2}, {fallback: 2},
		{music: future, fallback: " title "}, {fallback: " title "},
		{music: future, fallback: " "}, {fallback: " "},
		{music: future, fallback: errors.New("marker")}, {fallback: errors.New("marker")},
	} {
		if got, err := ensureAccessibleMusic(tt.music, now, tt.fallback, false); err == nil || got != nil {
			t.Errorf("ensure inaccessible (%#v, %#v) = %#v, %v", tt.music, tt.fallback, got, err)
		}
	}
	if filterAccessibleMusics(nil, now, false) != nil {
		t.Fatal("nil accessible filter should remain nil")
	}
	if got := filterAccessibleMusics([]*masterdata.Music{nil, hidden, future, visible}, now, false); len(got) != 1 || got[0] != visible {
		t.Fatalf("filtered music = %#v", got)
	}
	if accessibleMusicsSortedByPublishedAt(nil, now, false) != nil {
		t.Fatal("nil source sorting should return nil")
	}
	source := &lookupTestSource{musics: map[int]*masterdata.Music{
		3: {ID: 3, PublishedAt: 100},
		2: {ID: 2, PublishedAt: 100},
		1: {ID: 1, PublishedAt: 50},
	}}
	if got := accessibleMusicsSortedByPublishedAt(source, now, false); len(got) != 3 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("sorted accessible music = %#v", got)
	}
	if musicListDisplayOrder(nil) != 0 || musicListDisplayOrder(visible) != 10 || musicListDisplayOrder(&masterdata.Music{ID: 2, PublishedAt: 20}) != 20 || musicListDisplayOrder(&masterdata.Music{ID: 3}) != 3 {
		t.Fatal("music display order mismatch")
	}
	if got := stringsTrimSpace(" \t\n value \r "); got != "value" {
		t.Fatalf("stringsTrimSpace() = %q", got)
	}
}
