package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
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
		{
			_, err := client.Music.Create().
				SetGameID(item.id).
				SetTitle(item.title).
				SetPronunciation(item.reading).
				SetAssetbundleName("music_asset").
				SetPublishedAt(item.publishedAt).
				SetServerRegion(item.region.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create music %d/%s: %v", item.id, item.region, err)
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
		{
			_, err := client.Eventmusic.Create().
				SetEventID(link.eventID).
				SetMusicID(link.musicID).
				SetSeq(link.seq).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create event-music link %+v: %v", link, err)
		}

	}
	for _, event := range []struct {
		id, startAt int64
		name        string
	}{
		{id: 20, startAt: 2000, name: "later event"},
		{id: 21, startAt: 1000, name: "primary event"},
	} {
		{
			_, err := client.Event.Create().
				SetGameID(event.id).
				SetEventType("marathon").
				SetName(event.name).
				SetStartAt(event.startAt).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create event %d: %v", event.id, err)
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
		{
			_, err := client.Musicdifficultie.Create().
				SetGameID(difficulty.id).
				SetMusicID(difficulty.musicID).
				SetMusicDifficulty(difficulty.name).
				SetPlayLevel(difficulty.level).
				SetTotalNoteCount(difficulty.notes).
				SetServerRegion(difficulty.region.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create difficulty %d: %v", difficulty.id, err)
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
		{
			_, err := builder.Save(ctx)
			testutil.Require(t, !(err != nil), "create vocal %d: %v", vocal.id, err)
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
		{
			_, err := client.Musictag.Create().
				SetGameID(tag.id).
				SetMusicID(tag.musicID).
				SetMusicTag(tag.value).
				SetSeq(tag.seq).
				SetServerRegion(tag.region.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create tag %d: %v", tag.id, err)
		}

	}
	{
		_, err := client.Outsidecharacter.Create().
			SetGameID(5).
			SetName(" Guest Singer ").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create outside character: %v", err)
	}

	for _, window := range []struct {
		id, musicID, startAt, endAt int64
	}{
		{id: 1, musicID: 100, startAt: 1000, endAt: 2000},
		{id: 2, musicID: 100, startAt: 3000, endAt: 4000},
	} {
		{
			_, err := client.Limitedtimemusic.Create().
				SetGameID(window.id).
				SetMusicID(window.musicID).
				SetStartAt(window.startAt).
				SetEndAt(window.endAt).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create limited-time music %d: %v", window.id, err)
		}

	}

	musics := p.musics
	{
		_, err := musics.Search(ctx, " ")
		testutil.RequireArgs(t, !(err == nil), "blank music search should fail")
	}
	{

		_, err := musics.GetByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetByID(0) should fail")
	}

	music, err := musics.GetByID(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetByID(100) = %+v, %v", music, err)
		testutil.Require(t, !(music.Title != "Alpha Song"), "GetByID(100) = %+v, %v", music, err)
	}

	music.Title = "mutated"
	{
		cached, err := musics.GetByID(ctx, 100)
		{
			testutil.Require(t, !(err != nil), "cached music = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Title != "Alpha Song"), "cached music = %+v, %v", cached, err)
		}
	}
	{

		_, err := musics.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing music should fail")
	}

	all := musics.GetAll(ctx)
	{
		testutil.Require(t, !(len(all) != 4), "GetAll() = %+v", all)
		testutil.Require(t, !(all[0].ID != 101), "GetAll() = %+v", all)
		testutil.Require(t, !(all[1].ID != 102), "GetAll() = %+v", all)
		testutil.Require(t, !(all[3].ID != 103), "GetAll() = %+v", all)
	}

	all[0].Title = "mutated"
	{
		cached := musics.GetAll(ctx)
		{
			testutil.Require(t, !(len(cached) != 4), "cached music list = %+v", cached)
			testutil.Require(t, !(cached[0].Title != "Beta Song"), "cached music list = %+v", cached)
		}
	}

	for _, query := range []string{"100", "alpha song", "arufa"} {
		{
			got, err := musics.Search(ctx, query)
			{
				testutil.Require(t, !(err != nil), "Search(%q) = %+v, %v", query, got, err)
				testutil.Require(t, !(got.ID != 100), "Search(%q) = %+v, %v", query, got, err)
			}
		}

	}
	{
		_, err := musics.Search(ctx, "no such music")
		testutil.RequireArgs(t, !(err == nil), "missing music search should fail")
	}
	{

		got, err := musics.GetByEventID(ctx, 20)
		{
			testutil.Require(t, !(err != nil), "GetByEventID(20) = %+v, %v", got, err)
			testutil.Require(t, !(got.ID != 101), "GetByEventID(20) = %+v, %v", got, err)
		}
	}
	{

		_, err := musics.GetByEventID(ctx, 999)
		testutil.RequireArgs(t, !(err == nil), "missing event music should fail")
	}
	{

		_, err := musics.GetLocalizedTitles(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "localized titles for zero ID should fail")
	}

	titles, err := musics.GetLocalizedTitles(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetLocalizedTitles(100) = %+v, %v", titles, err)
		testutil.Require(t, !(len(titles) != 3), "GetLocalizedTitles(100) = %+v, %v", titles, err)
	}

	titles[0] = "mutated"
	{
		cached, err := musics.GetLocalizedTitles(ctx, 100)
		{
			testutil.Require(t, !(err != nil), "cached localized titles = %+v, %v", cached, err)
			testutil.Require(t, !(cached[0] == "mutated"), "cached localized titles = %+v, %v", cached, err)
		}
	}

	difficulties, err := musics.GetDifficulties(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetDifficulties(100) = %+v, %v", difficulties, err)
		testutil.Require(t, !(len(difficulties) != 2), "GetDifficulties(100) = %+v, %v", difficulties, err)
		testutil.Require(t, !(difficulties[0].MusicDifficulty != "expert"), "GetDifficulties(100) = %+v, %v", difficulties, err)
	}

	difficulties[0].PlayLevel = -1
	{
		cached, err := musics.GetDifficulties(ctx, 100)
		{
			testutil.Require(t, !(err != nil), "cached difficulties = %+v, %v", cached, err)
			testutil.Require(t, !(cached[0].PlayLevel != 26), "cached difficulties = %+v, %v", cached, err)
		}
	}
	{

		_, err := musics.GetDifficulties(ctx, 999)
		testutil.RequireArgs(t, !(err == nil), "missing difficulties should fail")
	}

	vocals, err := musics.GetVocals(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetVocals(100) = %+v, %v", vocals, err)
		testutil.Require(t, !(len(vocals) != 2), "GetVocals(100) = %+v, %v", vocals, err)
		testutil.Require(t, !(vocals[0].ID != 11), "GetVocals(100) = %+v, %v", vocals, err)
		testutil.Require(t, !(len(vocals[1].Characters) != 2), "GetVocals(100) = %+v, %v", vocals, err)
	}
	{

		_, err := musics.GetVocals(ctx, 999)
		testutil.RequireArgs(t, !(err == nil), "missing vocals should fail")
	}

	tags, err := musics.GetTags(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetTags(100) = %+v, %v", tags, err)
		testutil.Require(t, !(len(tags) != 1), "GetTags(100) = %+v, %v", tags, err)
		testutil.Require(t, !(tags[0] != "mv"), "GetTags(100) = %+v, %v", tags, err)
	}
	{

		tags, err := musics.GetTags(ctx, 999)
		{
			testutil.Require(t, !(err != nil), "missing tags = %+v, %v", tags, err)
			testutil.Require(t, !(len(tags) != 0), "missing tags = %+v, %v", tags, err)
		}
	}
	{

		_, err := musics.GetOutsideCharacterByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "outside character zero ID should fail")
	}
	{

		name, err := musics.GetOutsideCharacterByID(ctx, 5)
		{
			testutil.Require(t, !(err != nil), "GetOutsideCharacterByID(5) = %q, %v", name, err)
			testutil.Require(t, !(name != "Guest Singer"), "GetOutsideCharacterByID(5) = %q, %v", name, err)
		}
	}
	{

		name, err := musics.GetOutsideCharacterByID(ctx, 5)
		{
			testutil.Require(t, !(err != nil), "cached outside character = %q, %v", name, err)
			testutil.Require(t, !(name != "Guest Singer"), "cached outside character = %q, %v", name, err)
		}
	}
	{

		_, err := musics.GetOutsideCharacterByID(ctx, 999)
		testutil.RequireArgs(t, !(err == nil), "missing outside character should fail")
	}

	event, err := musics.GetPrimaryEventByMusicID(ctx, 100)
	{
		testutil.Require(t, !(err != nil), "GetPrimaryEventByMusicID(100) = %+v, %v", event, err)
		testutil.Require(t, !(event.ID != 21), "GetPrimaryEventByMusicID(100) = %+v, %v", event, err)
	}
	{

		_, err := musics.GetPrimaryEventByMusicID(ctx, 999)
		testutil.RequireArgs(t, !(err == nil), "music without event links should fail")
	}
	{

		_, err := musics.GetPrimaryEventByMusicID(ctx, 103)
		testutil.RequireArgs(t, !(err == nil), "music linked only to a missing event should fail")
	}

	windows := musics.GetLimitedTimeMusics(ctx, 100)
	{
		testutil.Require(t, !(len(windows) != 2), "GetLimitedTimeMusics(100) = %+v", windows)
		testutil.Require(t, !(windows[0].StartAt != 1000), "GetLimitedTimeMusics(100) = %+v", windows)
	}

	windows[0].StartAt = -1
	{
		cached := musics.GetLimitedTimeMusics(ctx, 100)
		{
			testutil.Require(t, !(len(cached) != 2), "cached limited-time musics = %+v", cached)
			testutil.Require(t, !(cached[0].StartAt != 1000), "cached limited-time musics = %+v", cached)
		}
	}
	{

		got := musics.GetLimitedTimeMusics(ctx, 999)
		testutil.Require(t, !(got != nil), "missing limited-time musics = %+v", got)
	}

}

func TestDBMusicProviderConversionAndQueryErrors(t *testing.T) {
	{
		testutil.RequireArgs(t, !(parseMusicVocalCharactersFromRaw(nil, 1, 2) != nil), "empty or malformed vocal characters should be nil")
		testutil.RequireArgs(t, !(parseMusicVocalCharactersFromRaw(json.RawMessage(`{`), 1, 2) != nil), "empty or malformed vocal characters should be nil")
	}

	parsed := parseMusicVocalCharactersFromRaw(json.RawMessage(`[{"characterType":"game","characterId":7},{"characterId":{}}]`), 10, 100)
	{
		testutil.Require(t, !(len(parsed) != 1), "parsed vocal characters = %+v", parsed)
		testutil.Require(t, !(parsed[0].ID != 1), "parsed vocal characters = %+v", parsed)
		testutil.Require(t, !(parsed[0].MusicVocalID != 10), "parsed vocal characters = %+v", parsed)
		testutil.Require(t, !(parsed[0].MusicID != 100), "parsed vocal characters = %+v", parsed)
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
		{
			testutil.Require(t, !(got != test.want), "interfaceToInt(%T) = %d, %v; want %d, %v", test.value, got, ok, test.want, test.ok)
			testutil.Require(t, !(ok != test.ok), "interfaceToInt(%T) = %d, %v; want %d, %v", test.value, got, ok, test.want, test.ok)
		}

	}
	{
		testutil.RequireArgs(t, !(cloneLimitedTimeMusics(nil) != nil), "cloning empty limited-time music data should stay empty")
		testutil.RequireArgs(t, !(len(cloneLimitedTimeMusics([]*masterdata.LimitedTimeMusic{nil})) != 0), "cloning empty limited-time music data should stay empty")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := openProviderBehaviorDB(t, "remaining_music_errors")
	musics := p.musics
	{
		_, err := musics.GetByID(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetByID should fail")
	}
	{

		got := musics.GetAll(ctx)
		testutil.Require(t, !(got != nil), "canceled GetAll = %+v", got)
	}
	{

		_, err := musics.GetByEventID(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetByEventID should fail")
	}
	{

		_, err := musics.GetLocalizedTitles(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetLocalizedTitles should fail")
	}
	{

		_, err := musics.GetDifficulties(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetDifficulties should fail")
	}
	{

		_, err := musics.GetVocals(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetVocals should fail")
	}
	{

		_, err := musics.GetTags(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetTags should fail")
	}
	{

		_, err := musics.GetOutsideCharacterByID(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetOutsideCharacterByID should fail")
	}
	{

		_, err := musics.GetPrimaryEventByMusicID(ctx, 1)
		testutil.RequireArgs(t, !(err == nil), "canceled GetPrimaryEventByMusicID should fail")
	}

}
