package music

import (
	"errors"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
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
	if nilService.WithTitleResolver(nil) != nil || nilService.WithAllowUnreleased(true) != nil {
		t.Fatal("nil search service options returned non-nil")
	}
	if _, err := nilService.SearchInfo(&QueryInfo{}); err == nil {
		t.Fatal("nil SearchInfo() error = nil")
	}
	if _, err := NewSearchService(nil, parser).SearchInfo(&QueryInfo{}); err == nil {
		t.Fatal("missing source SearchInfo() error = nil")
	}
	source := newRound4SearchSource()
	service := NewSearchService(source, parser)
	if service.WithAllowUnreleased(true) != service || service.WithTitleResolver(nil) != service {
		t.Fatal("search options did not return receiver")
	}
	if _, err := service.SearchInfo(nil); err == nil {
		t.Fatal("nil query info error = nil")
	}
	if _, err := service.Search(""); err == nil {
		t.Fatal("empty Search() error = nil")
	}
	if _, _, err := service.SearchChart(""); err == nil {
		t.Fatal("empty SearchChart() error = nil")
	}

	want := &masterdata.Music{ID: 7, Title: "Fallback", PublishedAt: 1}
	source.getByID = func(int) (*masterdata.Music, error) { return nil, errors.New("id lookup failed") }
	service.WithTitleResolver(func(query string) (*masterdata.Music, error) {
		if query != "Fallback" {
			t.Fatalf("fallback query = %q", query)
		}
		return want, nil
	})
	got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeID, Value: 99, Keyword: " Fallback ", AllowTitleFallback: true})
	if err != nil || got != want {
		t.Fatalf("ID title fallback = %#v, %v", got, err)
	}

	wantErr := errors.New("get by id failed")
	source.getByID = func(int) (*masterdata.Music, error) { return nil, wantErr }
	service.WithTitleResolver(nil)
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeID, Value: 8}); !errors.Is(err, wantErr) {
		t.Fatalf("ID source error = %v", err)
	}
	source.getByID = func(int) (*masterdata.Music, error) { return nil, nil }
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeID, Value: 8}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ID not-found error = %v", err)
	}
}

func TestSearchServiceSequenceEventBanAndTitleBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	parser := NewParser(nil)
	source := newRound4SearchSource()
	service := NewSearchService(source, parser)

	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeSeq, Value: 1}); err == nil || !strings.Contains(err.Error(), "no music") {
		t.Fatalf("empty sequence error = %v", err)
	}
	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "First", PublishedAt: now - 2},
		2: {ID: 2, Title: "Second", PublishedAt: now - 1},
	}
	if got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeSeq, Value: -1}); err != nil || got.ID != 2 {
		t.Fatalf("negative sequence = %#v, %v", got, err)
	}
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeSeq, Value: 3}); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("sequence range error = %v", err)
	}

	wantErr := errors.New("event lookup failed")
	source.getByEvent = func(int) (*masterdata.Music, error) { return nil, wantErr }
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeEvent, Value: 5}); !errors.Is(err, wantErr) {
		t.Fatalf("event source error = %v", err)
	}
	source.getByEvent = func(int) (*masterdata.Music, error) {
		return &masterdata.Music{ID: 5, PublishedAt: now + 100_000}, nil
	}
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeEvent, Value: 5}); err == nil {
		t.Fatal("unreleased event music error = nil")
	}

	fallback := &masterdata.Music{ID: 9, Title: "Ban fallback", PublishedAt: 1}
	service.WithTitleResolver(func(string) (*masterdata.Music, error) { return fallback, nil })
	if got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, Keyword: "alias", BanCharID: 1, BanSeq: 1}); err != nil || got != fallback {
		t.Fatalf("ban title fallback = %#v, %v", got, err)
	}
	ambiguous := &musicAmbiguousQueryError{sourceName: "alias", candidates: []musicQueryCandidate{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}}
	service.WithTitleResolver(func(string) (*masterdata.Music, error) { return nil, ambiguous })
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, Keyword: "alias", BanCharID: 1, BanSeq: 1}); !errors.Is(err, ambiguous) {
		t.Fatalf("ban ambiguous error = %v", err)
	}
	service.WithTitleResolver(nil)
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, BanCharID: 1, BanSeq: 1}); err == nil || !strings.Contains(err.Error(), "no ban events") {
		t.Fatalf("missing ban events error = %v", err)
	}
	source.banEvents = []*masterdata.Event{{ID: 50}}
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, BanCharID: 1, BanSeq: 2}); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("ban range error = %v", err)
	}
	source.getByEvent = func(int) (*masterdata.Music, error) { return nil, wantErr }
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeBan, BanCharID: 1, BanSeq: 1}); !errors.Is(err, wantErr) {
		t.Fatalf("ban event lookup error = %v", err)
	}

	source.getByID = func(id int) (*masterdata.Music, error) {
		return &masterdata.Music{ID: id, Title: "Direct", PublishedAt: 1}, nil
	}
	if got, err := service.SearchInfo(&QueryInfo{Type: QueryTypeChart, MusicID: 3}); err != nil || got.ID != 3 {
		t.Fatalf("chart ID lookup = %#v, %v", got, err)
	}
	source.getByID = func(int) (*masterdata.Music, error) { return nil, errors.New("missing") }
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeTitle, MusicID: 3}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("empty title with ID error = %v", err)
	}
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeTitle}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty title error = %v", err)
	}
	if _, err := service.SearchInfo(&QueryInfo{Type: QueryTypeUnknown}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported type error = %v", err)
	}
	if _, err := service.resolveTitle(" "); err == nil {
		t.Fatal("empty resolveTitle() error = nil")
	}
	source.search = func(string) (*masterdata.Music, error) { return nil, wantErr }
	if _, err := service.resolveTitle("song"); !errors.Is(err, wantErr) {
		t.Fatalf("resolveTitle source error = %v", err)
	}
}

func TestBuilderMetadataFallbackAndEdgeBranches(t *testing.T) {
	primary := newRound4SearchSource()
	fallback := newRound4SearchSource()
	builder := NewBuilder(primary, fallback, assets.NewAssetHelper("", nil))

	primary.vocalsErr = errors.New("primary vocals unavailable")
	fallback.vocalsErr = errors.New("fallback vocals unavailable")
	vocalInfo, err := builder.buildVocalInfo(1, renderregion.CN)
	if err != nil || len(vocalInfo.VocalInfo) != 0 || len(vocalInfo.VocalAssets) != 0 {
		t.Fatalf("empty vocal fallback = %#v, %v", vocalInfo, err)
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
	if err != nil || len(vocalInfo.VocalInfo) != 1 || vocalInfo.VocalAssets["初音未来"] == "" {
		t.Fatalf("populated fallback vocals = %#v, %v", vocalInfo, err)
	}

	primary.outside = map[int]string{3: " Primary Outside "}
	if name, avatar := builder.lookupVocalCharacter(masterdata.MusicVocalCharacter{CharacterType: "outside_character", CharacterID: 3}); name != "Primary Outside" || avatar {
		t.Fatalf("primary outside lookup = %q, %v", name, avatar)
	}
	if name, avatar := builder.lookupVocalCharacter(masterdata.MusicVocalCharacter{CharacterType: "outside_character", CharacterID: 999}); name != "" || avatar {
		t.Fatalf("missing outside lookup = %q, %v", name, avatar)
	}
	if got := builder.lookupCharacterName(0); got != "" {
		t.Fatalf("zero character name = %q", got)
	}
	primary.characters = map[int]*masterdata.Character{4: {ID: 4, FirstName: " A ", GivenName: "B "}}
	if got := builder.lookupCharacterName(4); got != "A B" {
		t.Fatalf("primary character name = %q", got)
	}
	if got := builder.lookupCharacterName(2); got != "初音未来" {
		t.Fatalf("fallback character name = %q", got)
	}
	if got := builder.lookupCharacterName(999); got != "" {
		t.Fatalf("missing character name = %q", got)
	}

	if got := builder.buildDisplayMusicTitle(nil, renderregion.CN); got != "" {
		t.Fatalf("nil display title = %q", got)
	}
	blankTitle := &masterdata.Music{ID: 1, Title: "  "}
	if got := builder.buildDisplayMusicTitle(blankTitle, renderregion.CN); got != "  " {
		t.Fatalf("blank display title = %q", got)
	}
	musicInfo := &masterdata.Music{ID: 1, Title: "Base"}
	if got := builder.buildDisplayMusicTitle(musicInfo, renderregion.JP); got != "Base" {
		t.Fatalf("JP display title = %q", got)
	}
	primary.localizedErr = errors.New("localized unavailable")
	if got := builder.buildDisplayMusicTitle(musicInfo, renderregion.CN); got != "Base" {
		t.Fatalf("localized error title = %q", got)
	}
	primary.localizedErr = nil
	primary.localized = []string{"Base", " "}
	if got := builder.buildDisplayMusicTitle(musicInfo, renderregion.CN); got != "Base" {
		t.Fatalf("empty localized title = %q", got)
	}
	primary.localized = []string{"中文标题"}
	if got := builder.buildDisplayMusicTitle(musicInfo, renderregion.CN); got != "Base (中文标题)" {
		t.Fatalf("localized display title = %q", got)
	}

	primary.musics[1] = &masterdata.Music{ID: 1, Categories: []string{"mv_3d"}}
	if got := builder.buildCategories(1); !reflectStrings(got, []string{"mv_3d"}) {
		t.Fatalf("music categories = %#v", got)
	}
	delete(primary.musics, 1)
	primary.tagsErr = errors.New("tags unavailable")
	if got := builder.buildCategories(1); got != nil {
		t.Fatalf("tag error categories = %#v", got)
	}
	primary.tagsErr = nil
	primary.tags = []string{"vocaloid", "mv"}
	if got := builder.buildCategories(1); !reflectStrings(got, primary.tags) {
		t.Fatalf("tag categories = %#v", got)
	}

	if got := builder.buildLimitedTimes(1, renderregion.JP); got != nil {
		t.Fatalf("empty limited times = %#v", got)
	}
	primary.limited = []*masterdata.LimitedTimeMusic{nil, {StartAt: 10, EndAt: 20}}
	if got := builder.buildLimitedTimes(1, renderregion.JP); len(got) != 1 || got[0][0] != 10 || got[0][1] != 20 {
		t.Fatalf("limited times = %#v", got)
	}
	if got := builder.buildEventBannerPath("event_asset", renderregion.JP); got == "" {
		t.Fatal("event banner path is empty")
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
	if _, err := (*Controller)(nil).ResolveMusicMetaRequests("jp", []string{"song"}); err == nil {
		t.Fatal("nil meta controller error = nil")
	}
	controller := NewController(nil, nil, nil, nil, nil)
	if _, err := controller.ResolveMusicMetaRequests("jp", nil); err == nil {
		t.Fatal("empty meta request error = nil")
	}
	if _, err := controller.ResolveMusicMetaRequests("jp", []string{"song"}); err == nil {
		t.Fatal("missing meta source error = nil")
	}
	source := newRound4SearchSource()
	controller = NewController(source, nil, nil, nil, nil)
	if _, err := controller.ResolveMusicMetaRequests("jp", []string{" ", "\t"}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("blank meta requests error = %v", err)
	}
	if _, err := controller.ResolveMusicMetaRequests("jp", []string{"missing"}); err == nil || !strings.Contains(err.Error(), "failed to resolve") {
		t.Fatalf("meta resolution error = %v", err)
	}
	source.musics[1] = &masterdata.Music{ID: 1, Title: "Song", PublishedAt: 1}
	if _, err := controller.ResolveMusicMetaRequests("jp", []string{"Song"}); err == nil || !strings.Contains(err.Error(), "no meta data") {
		t.Fatalf("missing meta data error = %v", err)
	}
	if _, err := controller.resolveMusicMetaQuery(source, " "); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty meta query error = %v", err)
	}
	if got := controller.resolveAllMusicMetas("jp", 0); got != nil {
		t.Fatalf("zero ID metas = %#v", got)
	}
}
