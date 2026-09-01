package music

import (
	"errors"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
)

type round4SearchSource struct {
	*lookupTestSource
	getByID       func(int) (*masterdata.Music, error)
	search        func(string) (*masterdata.Music, error)
	getByEvent    func(int) (*masterdata.Music, error)
	allMusics     []*masterdata.Music
	banEvents     []*masterdata.Event
	localized     []string
	localizedErr  error
	vocals        []*masterdata.MusicVocal
	vocalsErr     error
	tags          []string
	tagsErr       error
	characters    map[int]*masterdata.Character
	outside       map[int]string
	primaryEvent  *masterdata.Event
	primaryErr    error
	limited       []*masterdata.LimitedTimeMusic
	difficulties  []*masterdata.MusicDifficulty
	difficultyErr error
}

func newRound4SearchSource() *round4SearchSource {
	return &round4SearchSource{lookupTestSource: &lookupTestSource{
		region:       renderregion.JP,
		musics:       make(map[int]*masterdata.Music),
		difficulties: make(map[int][]*masterdata.MusicDifficulty),
	}}
}

func (s *round4SearchSource) SearchMusic(query string) (*masterdata.Music, error) {
	if s.search != nil {
		return s.search(query)
	}
	return s.lookupTestSource.SearchMusic(query)
}

func (s *round4SearchSource) GetMusicByID(id int) (*masterdata.Music, error) {
	if s.getByID != nil {
		return s.getByID(id)
	}
	return s.lookupTestSource.GetMusicByID(id)
}

func (s *round4SearchSource) GetMusicByEventID(id int) (*masterdata.Music, error) {
	if s.getByEvent != nil {
		return s.getByEvent(id)
	}
	return s.lookupTestSource.GetMusicByEventID(id)
}

func (s *round4SearchSource) GetMusics() []*masterdata.Music {
	if s.allMusics == nil {
		return s.lookupTestSource.GetMusics()
	}
	return append([]*masterdata.Music(nil), s.allMusics...)
}

func (s *round4SearchSource) GetBanEvents(int) []*masterdata.Event {
	return append([]*masterdata.Event(nil), s.banEvents...)
}

func (s *round4SearchSource) GetMusicLocalizedTitles(int) ([]string, error) {
	return append([]string(nil), s.localized...), s.localizedErr
}

func (s *round4SearchSource) GetMusicDifficulties(int) ([]*masterdata.MusicDifficulty, error) {
	if s.difficultyErr != nil || s.difficulties != nil {
		return append([]*masterdata.MusicDifficulty(nil), s.difficulties...), s.difficultyErr
	}
	return nil, nil
}

func (s *round4SearchSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) {
	return append([]*masterdata.MusicVocal(nil), s.vocals...), s.vocalsErr
}

func (s *round4SearchSource) GetMusicTags(int) ([]string, error) {
	return append([]string(nil), s.tags...), s.tagsErr
}

func (s *round4SearchSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	if item := s.characters[id]; item != nil {
		return new(*item), nil
	}
	return nil, errNotFound("character")
}

func (s *round4SearchSource) GetOutsideCharacterByID(id int) (string, error) {
	if name, ok := s.outside[id]; ok {
		return name, nil
	}
	return "", errNotFound("outside character")
}

func (s *round4SearchSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic {
	return append([]*masterdata.LimitedTimeMusic(nil), s.limited...)
}

func (s *round4SearchSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return s.primaryEvent, s.primaryErr
}

func TestSearchServiceValidationAndIDBranches(t *testing.T) {
	parser := NewParser(nil)
	var nilService *SearchService
	{
		testutil.RequireArgs(t, !(nilService.WithTitleResolver(nil) != nil), "nil search service options returned non-nil")
		testutil.RequireArgs(t, !(nilService.WithAllowUnreleased(true) != nil), "nil search service options returned non-nil")
	}
	{

		_, err := nilService.SearchInfo(&QueryInfo{})
		testutil.RequireArgs(t, !(err == nil), "nil SearchInfo() error = nil")
	}
	{

		_, err := NewSearchService(nil, parser).SearchInfo(&QueryInfo{})
		testutil.RequireArgs(t, !(err == nil), "missing source SearchInfo() error = nil")
	}

	source := newRound4SearchSource()
	service := NewSearchService(source, parser)
	{
		testutil.RequireArgs(t, !(service.WithAllowUnreleased(true) != service), "search options did not return receiver")
		testutil.RequireArgs(t, !(service.WithTitleResolver(nil) != service), "search options did not return receiver")
	}
	{

		_, err := service.SearchInfo(nil)
		testutil.RequireArgs(t, !(err == nil), "nil query info error = nil")
	}
	{

		_, err := service.Search("")
		testutil.RequireArgs(t, !(err == nil), "empty Search() error = nil")
	}
	{

		_, _, err := service.SearchChart("")
		testutil.RequireArgs(t, !(err == nil), "empty SearchChart() error = nil")
	}

	want := &masterdata.Music{ID: 7, Title: "Fallback", PublishedAt: 1}
	source.getByID = func(int) (*masterdata.Music, error) { return nil, errors.New("id lookup failed") }
	service.WithTitleResolver(func(query string) (*masterdata.Music, error) {
		testutil.Require(t, !(query != "Fallback"), "fallback query = %q", query)

		return want, nil
	})
	got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeID, Value: 99, Keyword: " Fallback ", AllowTitleFallback: true})
	{
		testutil.Require(t, !(err != nil), "ID title fallback = %#v, %v", got, err)
		testutil.Require(t, !(got != want), "ID title fallback = %#v, %v", got, err)
	}

	wantErr := errors.New("get by id failed")
	source.getByID = func(int) (*masterdata.Music, error) { return nil, wantErr }
	service.WithTitleResolver(nil)
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeID, Value: 8})
		testutil.Require(t, errors.Is(err, wantErr), "ID source error = %v", err)
	}

	source.getByID = func(int) (*masterdata.Music, error) { return nil, nil }
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeID, Value: 8})
		{
			testutil.Require(t, !(err == nil), "ID not-found error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not found"), "ID not-found error = %v", err)
		}
	}

}

func TestSearchServiceSequenceEventBanAndTitleBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	parser := NewParser(nil)
	source := newRound4SearchSource()
	service := NewSearchService(source, parser)
	{

		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeSeq, Value: 1})
		{
			testutil.Require(t, !(err == nil), "empty sequence error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "no music"), "empty sequence error = %v", err)
		}
	}

	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "First", PublishedAt: now - 2},
		2: {ID: 2, Title: "Second", PublishedAt: now - 1},
	}
	{
		got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeSeq, Value: -1})
		{
			testutil.Require(t, !(err != nil), "negative sequence = %#v, %v", got, err)
			testutil.Require(t, !(got.ID != 2), "negative sequence = %#v, %v", got, err)
		}
	}
	{

		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeSeq, Value: 3})
		{
			testutil.Require(t, !(err == nil), "sequence range error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "out of range"), "sequence range error = %v", err)
		}
	}

	wantErr := errors.New("event lookup failed")
	source.getByEvent = func(int) (*masterdata.Music, error) { return nil, wantErr }
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeEvent, Value: 5})
		testutil.Require(t, errors.Is(err, wantErr), "event source error = %v", err)
	}

	source.getByEvent = func(int) (*masterdata.Music, error) {
		return &masterdata.Music{ID: 5, PublishedAt: now + 100_000}, nil
	}
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeEvent, Value: 5})
		testutil.RequireArgs(t, !(err == nil), "unreleased event music error = nil")
	}

	fallback := &masterdata.Music{ID: 9, Title: "Ban fallback", PublishedAt: 1}
	service.WithTitleResolver(func(string) (*masterdata.Music, error) { return fallback, nil })
	{
		got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, Keyword: "alias", BanCharID: 1, BanSeq: 1})
		{
			testutil.Require(t, !(err != nil), "ban title fallback = %#v, %v", got, err)
			testutil.Require(t, !(got != fallback), "ban title fallback = %#v, %v", got, err)
		}
	}

	ambiguous := &musicAmbiguousQueryError{sourceName: "alias", candidates: []musicQueryCandidate{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}}
	service.WithTitleResolver(func(string) (*masterdata.Music, error) { return nil, ambiguous })
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, Keyword: "alias", BanCharID: 1, BanSeq: 1})
		testutil.Require(t, errors.Is(err, ambiguous), "ban ambiguous error = %v", err)
	}

	service.WithTitleResolver(nil)
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, BanCharID: 1, BanSeq: 1})
		{
			testutil.Require(t, !(err == nil), "missing ban events error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "no ban events"), "missing ban events error = %v", err)
		}
	}

	source.banEvents = []*masterdata.Event{{ID: 50}}
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, BanCharID: 1, BanSeq: 2})
		{
			testutil.Require(t, !(err == nil), "ban range error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "out of range"), "ban range error = %v", err)
		}
	}

	source.getByEvent = func(int) (*masterdata.Music, error) { return nil, wantErr }
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, BanCharID: 1, BanSeq: 1})
		testutil.Require(t, errors.Is(err, wantErr), "ban event lookup error = %v", err)
	}

	source.getByID = func(id int) (*masterdata.Music, error) {
		return &masterdata.Music{ID: id, Title: "Direct", PublishedAt: 1}, nil
	}
	{
		got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeChart, MusicID: 3})
		{
			testutil.Require(t, !(err != nil), "chart ID lookup = %#v, %v", got, err)
			testutil.Require(t, !(got.ID != 3), "chart ID lookup = %#v, %v", got, err)
		}
	}

	source.getByID = func(int) (*masterdata.Music, error) { return nil, errors.New("missing") }
	{
		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeTitle, MusicID: 3})
		{
			testutil.Require(t, !(err == nil), "empty title with ID error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not found"), "empty title with ID error = %v", err)
		}
	}
	{

		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeTitle})
		{
			testutil.Require(t, !(err == nil), "empty title error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "empty"), "empty title error = %v", err)
		}
	}
	{

		_, err := service.SearchInfo(&QueryInfo{Type: QueryTypeUnknown})
		{
			testutil.Require(t, !(err == nil), "unsupported type error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "unsupported"), "unsupported type error = %v", err)
		}
	}
	{

		_, err := service.resolveTitle(" ")
		testutil.RequireArgs(t, !(err == nil), "empty resolveTitle() error = nil")
	}

	source.search = func(string) (*masterdata.Music, error) { return nil, wantErr }
	{
		_, err := service.resolveTitle("song")
		testutil.Require(t, errors.Is(err, wantErr), "resolveTitle source error = %v", err)
	}

}

func TestBuilderMetadataFallbackAndEdgeBranches(t *testing.T) {
	primary := newRound4SearchSource()
	fallback := newRound4SearchSource()
	builder := NewBuilder(primary, fallback, assets.NewAssetHelper("", nil))

	primary.vocalsErr = errors.New("primary vocals unavailable")
	fallback.vocalsErr = errors.New("fallback vocals unavailable")
	vocalInfo, err := builder.buildVocalInfo(1, renderregion.CN)
	{
		testutil.Require(t, !(err != nil), "empty vocal fallback = %#v, %v", vocalInfo, err)
		testutil.Require(t, !(len(vocalInfo.VocalInfo) != 0), "empty vocal fallback = %#v, %v", vocalInfo, err)
		testutil.Require(t, !(len(vocalInfo.VocalAssets) != 0), "empty vocal fallback = %#v, %v", vocalInfo, err)
	}

	primary.vocalsErr = errors.New("primary vocals unavailable")
	fallback.vocalsErr = nil
	fallback.vocals = []*masterdata.MusicVocal{
		nil,
		{
			MusicVocalType: "sekai", Caption: "セカイver.", AssetBundleName: "vocal_asset",
			Characters: []masterdata.MusicVocalCharacter{
				{CharacterType: "outside_character", CharacterID: 1},
				{CharacterType: "game_character", CharacterID: 2},
			},
		},
	}
	fallback.outside = map[int]string{1: " Outside "}
	fallback.characters = map[int]*masterdata.Character{2: {ID: 2, FirstName: "初音", GivenName: "未来"}}
	vocalInfo, err = builder.buildVocalInfo(1, renderregion.CN)
	{
		testutil.Require(t, !(err != nil), "populated fallback vocals = %#v, %v", vocalInfo, err)
		testutil.Require(t, !(len(vocalInfo.VocalInfo) != 1), "populated fallback vocals = %#v, %v", vocalInfo, err)
		testutil.Require(t, !(vocalInfo.VocalAssets["初音未来"] == ""), "populated fallback vocals = %#v, %v", vocalInfo, err)
	}

	primary.outside = map[int]string{3: " Primary Outside "}
	{
		name, avatar := builder.lookupVocalCharacter(masterdata.MusicVocalCharacter{CharacterType: "outside_character", CharacterID: 3})
		{
			testutil.Require(t, !(name != "Primary Outside"), "primary outside lookup = %q, %v", name, avatar)
			testutil.Require(t, !(avatar), "primary outside lookup = %q, %v", name, avatar)
		}
	}
	{

		name, avatar := builder.lookupVocalCharacter(masterdata.MusicVocalCharacter{CharacterType: "outside_character", CharacterID: 999})
		{
			testutil.Require(t, !(name != ""), "missing outside lookup = %q, %v", name, avatar)
			testutil.Require(t, !(avatar), "missing outside lookup = %q, %v", name, avatar)
		}
	}
	{

		got := builder.lookupCharacterName(0)
		testutil.Require(t, !(got != ""), "zero character name = %q", got)
	}

	primary.characters = map[int]*masterdata.Character{4: {ID: 4, FirstName: " A ", GivenName: "B "}}
	{
		got := builder.lookupCharacterName(4)
		testutil.Require(t, !(got != "A B"), "primary character name = %q", got)
	}
	{

		got := builder.lookupCharacterName(2)
		testutil.Require(t, !(got != "初音未来"), "fallback character name = %q", got)
	}
	{

		got := builder.lookupCharacterName(999)
		testutil.Require(t, !(got != ""), "missing character name = %q", got)
	}
	{

		got := builder.buildDisplayMusicTitle(nil, renderregion.CN)
		testutil.Require(t, !(got != ""), "nil display title = %q", got)
	}

	blankTitle := &masterdata.Music{ID: 1, Title: "  "}
	{
		got := builder.buildDisplayMusicTitle(blankTitle, renderregion.CN)
		testutil.Require(t, !(got != "  "), "blank display title = %q", got)
	}

	musicInfo := &masterdata.Music{ID: 1, Title: "Base"}
	{
		got := builder.buildDisplayMusicTitle(musicInfo, renderregion.JP)
		testutil.Require(t, !(got != "Base"), "JP display title = %q", got)
	}

	primary.localizedErr = errors.New("localized unavailable")
	{
		got := builder.buildDisplayMusicTitle(musicInfo, renderregion.CN)
		testutil.Require(t, !(got != "Base"), "localized error title = %q", got)
	}

	primary.localizedErr = nil
	primary.localized = []string{"Base", " "}
	{
		got := builder.buildDisplayMusicTitle(musicInfo, renderregion.CN)
		testutil.Require(t, !(got != "Base"), "empty localized title = %q", got)
	}

	primary.localized = []string{"中文标题"}
	{
		got := builder.buildDisplayMusicTitle(musicInfo, renderregion.CN)
		testutil.Require(t, !(got != "Base (中文标题)"), "localized display title = %q", got)
	}

	primary.musics[1] = &masterdata.Music{ID: 1, Categories: []string{"mv_3d"}}
	{
		got := builder.buildCategories(1)
		testutil.Require(t, reflectStrings(got, []string{"mv_3d"}), "music categories = %#v", got)
	}

	delete(primary.musics, 1)
	primary.tagsErr = errors.New("tags unavailable")
	{
		got := builder.buildCategories(1)
		testutil.Require(t, !(got != nil), "tag error categories = %#v", got)
	}

	primary.tagsErr = nil
	primary.tags = []string{"vocaloid", "mv"}
	{
		got := builder.buildCategories(1)
		testutil.Require(t, reflectStrings(got, primary.tags), "tag categories = %#v", got)
	}
	{

		got := builder.buildLimitedTimes(1, renderregion.JP)
		testutil.Require(t, !(got != nil), "empty limited times = %#v", got)
	}

	primary.limited = []*masterdata.LimitedTimeMusic{nil, {StartAt: 10, EndAt: 20}}
	{
		got := builder.buildLimitedTimes(1, renderregion.JP)
		{
			testutil.Require(t, !(len(got) != 1), "limited times = %#v", got)
			testutil.Require(t, !(got[0][0] != 10), "limited times = %#v", got)
			testutil.Require(t, !(got[0][1] != 20), "limited times = %#v", got)
		}
	}
	{

		got := builder.buildEventBannerPath("event_asset", renderregion.JP)
		testutil.RequireArgs(t, !(got == ""), "event banner path is empty")
	}

}

func reflectStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestMusicMetaRequestValidationBranches(t *testing.T) {
	{
		_, err := (*Controller)(nil).ResolveMusicMetaRequests("jp", []string{"song"})
		testutil.RequireArgs(t, !(err == nil), "nil meta controller error = nil")
	}

	controller := NewController(nil, nil, nil, nil, nil)
	{
		_, err := controller.ResolveMusicMetaRequests("jp", nil)
		testutil.RequireArgs(t, !(err == nil), "empty meta request error = nil")
	}
	{

		_, err := controller.ResolveMusicMetaRequests("jp", []string{"song"})
		testutil.RequireArgs(t, !(err == nil), "missing meta source error = nil")
	}

	source := newRound4SearchSource()
	controller = NewController(source, nil, nil, nil, nil)
	{
		_, err := controller.ResolveMusicMetaRequests("jp", []string{" ", "\t"})
		{
			testutil.Require(t, !(err == nil), "blank meta requests error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "empty"), "blank meta requests error = %v", err)
		}
	}
	{

		_, err := controller.ResolveMusicMetaRequests("jp", []string{"missing"})
		{
			testutil.Require(t, !(err == nil), "meta resolution error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "failed to resolve"), "meta resolution error = %v", err)
		}
	}

	source.musics[1] = &masterdata.Music{ID: 1, Title: "Song", PublishedAt: 1}
	{
		_, err := controller.ResolveMusicMetaRequests("jp", []string{"Song"})
		{
			testutil.Require(t, !(err == nil), "missing meta data error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "no meta data"), "missing meta data error = %v", err)
		}
	}
	{

		_, err := controller.resolveMusicMetaQuery(source, " ")
		{
			testutil.Require(t, !(err == nil), "empty meta query error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "empty"), "empty meta query error = %v", err)
		}
	}
	{

		got := controller.resolveAllMusicMetas("jp", 0)
		testutil.Require(t, !(got != nil), "zero ID metas = %#v", got)
	}

}
