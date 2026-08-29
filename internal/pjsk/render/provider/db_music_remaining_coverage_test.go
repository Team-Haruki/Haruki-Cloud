package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestDBMusicProviderCoreAndDetailQueries(t *testing.T) {
	ctx := context.Background()
	p := openProviderBehaviorDB(t, "remaining_music")
	client := p.client

	for _, item := range []struct {
		id, publishedAt int64
		title, reading  string
		region          renderregion.Value
	}{
		{id: 100, publishedAt: 200, title: "Alpha Song", reading: "Arufa", region: renderregion.JP},
		{id: 101, publishedAt: 100, title: "Beta Song", reading: "Beta", region: renderregion.JP},
		{id: 102, publishedAt: 100, title: "Gamma Song", reading: "Gamma", region: renderregion.JP},
		{id: 103, publishedAt: 300, title: "Missing Event Song", reading: "Missing", region: renderregion.JP},
		{id: 100, publishedAt: 200, title: "Localized Alpha", reading: "Arufa", region: renderregion.TW},
	} {
		if _, err := client.Music.Create().
			SetGameID(item.id).
			SetTitle(item.title).
			SetPronunciation(item.reading).
			SetAssetbundleName("music_asset").
			SetPublishedAt(item.publishedAt).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create music %d/%s: %v", item.id, item.region, err)
		}
	}
	for _, link := range []struct {
		eventID, musicID, seq int64
	}{
		{eventID: 20, musicID: 100, seq: 2},
		{eventID: 20, musicID: 101, seq: 1},
		{eventID: 21, musicID: 100, seq: 3},
		{eventID: 404, musicID: 103, seq: 1},
	} {
		if _, err := client.Eventmusic.Create().
			SetEventID(link.eventID).
			SetMusicID(link.musicID).
			SetSeq(link.seq).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create event-music link %+v: %v", link, err)
		}
	}
	for _, event := range []struct {
		id, startAt int64
		name        string
	}{
		{id: 20, startAt: 2000, name: "later event"},
		{id: 21, startAt: 1000, name: "primary event"},
	} {
		if _, err := client.Event.Create().
			SetGameID(event.id).
			SetEventType("marathon").
			SetName(event.name).
			SetStartAt(event.startAt).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create event %d: %v", event.id, err)
		}
	}
	for _, difficulty := range []struct {
		id, musicID, level, notes int64
		name                      string
		region                    renderregion.Value
	}{
		{id: 1, musicID: 100, level: 26, notes: 800, name: "expert", region: renderregion.JP},
		{id: 2, musicID: 100, level: 30, notes: 1000, name: "master", region: renderregion.JP},
		{id: 3, musicID: 100, level: 31, notes: 1100, name: "append", region: renderregion.TW},
	} {
		if _, err := client.Musicdifficultie.Create().
			SetGameID(difficulty.id).
			SetMusicID(difficulty.musicID).
			SetMusicDifficulty(difficulty.name).
			SetPlayLevel(difficulty.level).
			SetTotalNoteCount(difficulty.notes).
			SetServerRegion(difficulty.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create difficulty %d: %v", difficulty.id, err)
		}
	}
	for _, vocal := range []struct {
		id, seq int64
		caption string
		chars   json.RawMessage
	}{
		{id: 10, seq: 2, caption: "solo", chars: json.RawMessage(`[{"characterType":"game_character","characterId":1},{"characterType":"outside","characterId":"2"},{"characterType":"bad","characterId":"not-a-number"}]`)},
		{id: 11, seq: 1, caption: "instrumental", chars: nil},
	} {
		builder := client.Musicvocal.Create().
			SetGameID(vocal.id).
			SetMusicID(100).
			SetMusicVocalType("sekai").
			SetSeq(vocal.seq).
			SetCaption(vocal.caption).
			SetAssetbundleName("vocal_asset").
			SetServerRegion(renderregion.JP.String())
		if vocal.chars != nil {
			builder.SetCharacters(vocal.chars)
		}
		if _, err := builder.Save(ctx); err != nil {
			t.Fatalf("create vocal %d: %v", vocal.id, err)
		}
	}
	for _, tag := range []struct {
		id, musicID, seq int64
		value            string
		region           renderregion.Value
	}{
		{id: 1, musicID: 100, seq: 2, value: " mv ", region: renderregion.JP},
		{id: 2, musicID: 100, seq: 1, value: " ", region: renderregion.JP},
		{id: 3, musicID: 100, seq: 3, value: "other", region: renderregion.TW},
	} {
		if _, err := client.Musictag.Create().
			SetGameID(tag.id).
			SetMusicID(tag.musicID).
			SetMusicTag(tag.value).
			SetSeq(tag.seq).
			SetServerRegion(tag.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create tag %d: %v", tag.id, err)
		}
	}
	if _, err := client.Outsidecharacter.Create().
		SetGameID(5).
		SetName(" Guest Singer ").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create outside character: %v", err)
	}
	for _, window := range []struct {
		id, musicID, startAt, endAt int64
	}{
		{id: 1, musicID: 100, startAt: 1000, endAt: 2000},
		{id: 2, musicID: 100, startAt: 3000, endAt: 4000},
	} {
		if _, err := client.Limitedtimemusic.Create().
			SetGameID(window.id).
			SetMusicID(window.musicID).
			SetStartAt(window.startAt).
			SetEndAt(window.endAt).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create limited-time music %d: %v", window.id, err)
		}
	}

	musics := p.musics
	if _, err := musics.Search(ctx, " "); err == nil {
		t.Fatal("blank music search should fail")
	}
	if _, err := musics.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should fail")
	}
	music, err := musics.GetByID(ctx, 100)
	if err != nil || music.Title != "Alpha Song" {
		t.Fatalf("GetByID(100) = %+v, %v", music, err)
	}
	music.Title = "mutated"
	if cached, err := musics.GetByID(ctx, 100); err != nil || cached.Title != "Alpha Song" {
		t.Fatalf("cached music = %+v, %v", cached, err)
	}
	if _, err := musics.GetByID(ctx, 404); err == nil {
		t.Fatal("missing music should fail")
	}
	all := musics.GetAll(ctx)
	if len(all) != 4 || all[0].ID != 101 || all[1].ID != 102 || all[3].ID != 103 {
		t.Fatalf("GetAll() = %+v", all)
	}
	all[0].Title = "mutated"
	if cached := musics.GetAll(ctx); len(cached) != 4 || cached[0].Title != "Beta Song" {
		t.Fatalf("cached music list = %+v", cached)
	}
	for _, query := range []string{"100", "alpha song", "arufa"} {
		if got, err := musics.Search(ctx, query); err != nil || got.ID != 100 {
			t.Fatalf("Search(%q) = %+v, %v", query, got, err)
		}
	}
	if _, err := musics.Search(ctx, "no such music"); err == nil {
		t.Fatal("missing music search should fail")
	}
	if got, err := musics.GetByEventID(ctx, 20); err != nil || got.ID != 101 {
		t.Fatalf("GetByEventID(20) = %+v, %v", got, err)
	}
	if _, err := musics.GetByEventID(ctx, 999); err == nil {
		t.Fatal("missing event music should fail")
	}
	if _, err := musics.GetLocalizedTitles(ctx, 0); err == nil {
		t.Fatal("localized titles for zero ID should fail")
	}
	titles, err := musics.GetLocalizedTitles(ctx, 100)
	if err != nil || len(titles) != 3 {
		t.Fatalf("GetLocalizedTitles(100) = %+v, %v", titles, err)
	}
	titles[0] = "mutated"
	if cached, err := musics.GetLocalizedTitles(ctx, 100); err != nil || cached[0] == "mutated" {
		t.Fatalf("cached localized titles = %+v, %v", cached, err)
	}

	difficulties, err := musics.GetDifficulties(ctx, 100)
	if err != nil || len(difficulties) != 2 || difficulties[0].MusicDifficulty != "expert" {
		t.Fatalf("GetDifficulties(100) = %+v, %v", difficulties, err)
	}
	difficulties[0].PlayLevel = -1
	if cached, err := musics.GetDifficulties(ctx, 100); err != nil || cached[0].PlayLevel != 26 {
		t.Fatalf("cached difficulties = %+v, %v", cached, err)
	}
	if _, err := musics.GetDifficulties(ctx, 999); err == nil {
		t.Fatal("missing difficulties should fail")
	}
	vocals, err := musics.GetVocals(ctx, 100)
	if err != nil || len(vocals) != 2 || vocals[0].ID != 11 || len(vocals[1].Characters) != 2 {
		t.Fatalf("GetVocals(100) = %+v, %v", vocals, err)
	}
	if _, err := musics.GetVocals(ctx, 999); err == nil {
		t.Fatal("missing vocals should fail")
	}
	tags, err := musics.GetTags(ctx, 100)
	if err != nil || len(tags) != 1 || tags[0] != "mv" {
		t.Fatalf("GetTags(100) = %+v, %v", tags, err)
	}
	if tags, err := musics.GetTags(ctx, 999); err != nil || len(tags) != 0 {
		t.Fatalf("missing tags = %+v, %v", tags, err)
	}
	if _, err := musics.GetOutsideCharacterByID(ctx, 0); err == nil {
		t.Fatal("outside character zero ID should fail")
	}
	if name, err := musics.GetOutsideCharacterByID(ctx, 5); err != nil || name != "Guest Singer" {
		t.Fatalf("GetOutsideCharacterByID(5) = %q, %v", name, err)
	}
	if name, err := musics.GetOutsideCharacterByID(ctx, 5); err != nil || name != "Guest Singer" {
		t.Fatalf("cached outside character = %q, %v", name, err)
	}
	if _, err := musics.GetOutsideCharacterByID(ctx, 999); err == nil {
		t.Fatal("missing outside character should fail")
	}
	event, err := musics.GetPrimaryEventByMusicID(ctx, 100)
	if err != nil || event.ID != 21 {
		t.Fatalf("GetPrimaryEventByMusicID(100) = %+v, %v", event, err)
	}
	if _, err := musics.GetPrimaryEventByMusicID(ctx, 999); err == nil {
		t.Fatal("music without event links should fail")
	}
	if _, err := musics.GetPrimaryEventByMusicID(ctx, 103); err == nil {
		t.Fatal("music linked only to a missing event should fail")
	}
	windows := musics.GetLimitedTimeMusics(ctx, 100)
	if len(windows) != 2 || windows[0].StartAt != 1000 {
		t.Fatalf("GetLimitedTimeMusics(100) = %+v", windows)
	}
	windows[0].StartAt = -1
	if cached := musics.GetLimitedTimeMusics(ctx, 100); len(cached) != 2 || cached[0].StartAt != 1000 {
		t.Fatalf("cached limited-time musics = %+v", cached)
	}
	if got := musics.GetLimitedTimeMusics(ctx, 999); got != nil {
		t.Fatalf("missing limited-time musics = %+v", got)
	}
}

func TestDBMusicProviderConversionAndQueryErrors(t *testing.T) {
	if parseMusicVocalCharactersFromRaw(nil, 1, 2) != nil || parseMusicVocalCharactersFromRaw(json.RawMessage(`{`), 1, 2) != nil {
		t.Fatal("empty or malformed vocal characters should be nil")
	}
	parsed := parseMusicVocalCharactersFromRaw(json.RawMessage(`[{"characterType":"game","characterId":7},{"characterId":{}}]`), 10, 100)
	if len(parsed) != 1 || parsed[0].ID != 1 || parsed[0].MusicVocalID != 10 || parsed[0].MusicID != 100 {
		t.Fatalf("parsed vocal characters = %+v", parsed)
	}
	for _, test := range []struct {
		value any
		want  int
		ok    bool
	}{
		{value: int(1), want: 1, ok: true},
		{value: int32(2), want: 2, ok: true},
		{value: int64(3), want: 3, ok: true},
		{value: float64(4), want: 4, ok: true},
		{value: json.Number("5"), want: 5, ok: true},
		{value: json.Number("bad"), want: 0, ok: false},
		{value: " 6 ", want: 6, ok: true},
		{value: "bad", want: 0, ok: false},
		{value: struct{}{}, want: 0, ok: false},
	} {
		got, ok := interfaceToInt(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("interfaceToInt(%T) = %d, %v; want %d, %v", test.value, got, ok, test.want, test.ok)
		}
	}
	if cloneLimitedTimeMusics(nil) != nil || len(cloneLimitedTimeMusics([]*masterdata.LimitedTimeMusic{nil})) != 0 {
		t.Fatal("cloning empty limited-time music data should stay empty")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := openProviderBehaviorDB(t, "remaining_music_errors")
	musics := p.musics
	if _, err := musics.GetByID(ctx, 1); err == nil {
		t.Fatal("canceled GetByID should fail")
	}
	if got := musics.GetAll(ctx); got != nil {
		t.Fatalf("canceled GetAll = %+v", got)
	}
	if _, err := musics.GetByEventID(ctx, 1); err == nil {
		t.Fatal("canceled GetByEventID should fail")
	}
	if _, err := musics.GetLocalizedTitles(ctx, 1); err == nil {
		t.Fatal("canceled GetLocalizedTitles should fail")
	}
	if _, err := musics.GetDifficulties(ctx, 1); err == nil {
		t.Fatal("canceled GetDifficulties should fail")
	}
	if _, err := musics.GetVocals(ctx, 1); err == nil {
		t.Fatal("canceled GetVocals should fail")
	}
	if _, err := musics.GetTags(ctx, 1); err == nil {
		t.Fatal("canceled GetTags should fail")
	}
	if _, err := musics.GetOutsideCharacterByID(ctx, 1); err == nil {
		t.Fatal("canceled GetOutsideCharacterByID should fail")
	}
	if _, err := musics.GetPrimaryEventByMusicID(ctx, 1); err == nil {
		t.Fatal("canceled GetPrimaryEventByMusicID should fail")
	}
}
