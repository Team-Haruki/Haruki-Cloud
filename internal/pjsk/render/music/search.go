package music

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func NewSearchService(source DataSource, parser *Parser) *SearchService {
	return &SearchService{source: source, parser: parser}
}

func (s *SearchService) WithTitleResolver(resolver func(string) (*masterdata.Music, error)) *SearchService {
	if s == nil {
		return nil
	}
	s.titleResolver = resolver
	return s
}

func (s *SearchService) WithAllowUnreleased(allow bool) *SearchService {
	if s == nil {
		return nil
	}
	s.allowUnreleased = allow
	return s
}

func (s *SearchService) Search(query string) (*masterdata.Music, error) {
	info, err := s.parser.Parse(query)
	if err != nil {
		return nil, err
	}
	return s.SearchInfo(info)
}

func (s *SearchService) SearchChart(query string) (*QueryInfo, *masterdata.Music, error) {
	info, err := s.parser.ParseChart(query)
	if err != nil {
		return nil, nil, err
	}
	musicInfo, err := s.SearchInfo(info)
	if err != nil {
		return nil, nil, err
	}
	return info, musicInfo, nil
}

func (s *SearchService) SearchInfo(info *QueryInfo) (*masterdata.Music, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("music data source is not configured")
	}
	if info == nil {
		return nil, fmt.Errorf("music query info is required")
	}

	now := currentMusicVisibilityTime()

	switch info.Type {
	case QueryTypeID:
		return s.searchByID(info, now)

	case QueryTypeSeq:
		return s.searchBySequence(info.Value, now)

	case QueryTypeEvent:
		return s.searchByEvent(info.Value, now)

	case QueryTypeBan:
		return s.searchByBan(info, now)

	case QueryTypeTitle, QueryTypeChart:
		return s.searchByTitleOrChart(info, now)

	default:
		return nil, fmt.Errorf("unsupported music query type: %d", info.Type)
	}
}

func (s *SearchService) searchByID(info *QueryInfo, now int64) (*masterdata.Music, error) {
	musicInfo, err := s.source.GetMusicByID(info.Value)
	if err == nil && musicInfo != nil {
		if isMusicAccessibleAt(musicInfo, now, s.allowUnreleased) {
			return musicInfo, nil
		}
		return nil, releasecheck.New(releasecheck.KindMusic, "", info.Value)
	}
	if info.AllowTitleFallback {
		if resolved := s.resolveTitleFallback(info.Keyword); resolved != nil {
			return resolved, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("music not found: %d", info.Value)
}

func (s *SearchService) resolveTitleFallback(keyword string) *masterdata.Music {
	fallback := strings.TrimSpace(keyword)
	if fallback == "" {
		return nil
	}
	resolved, err := s.resolveTitle(fallback)
	if err != nil {
		return nil
	}
	return resolved
}

func (s *SearchService) searchBySequence(sequence int, now int64) (*masterdata.Music, error) {
	// Sequence lookup stays anchored to songs released in the target region.
	musics := accessibleMusicsSortedByPublishedAt(s.source, now, false)
	if len(musics) == 0 {
		return nil, fmt.Errorf("no music data available")
	}
	index := sequence - 1
	if sequence < 0 {
		index = len(musics) + sequence
	}
	if index < 0 || index >= len(musics) {
		return nil, fmt.Errorf("music index out of range: %d", sequence)
	}
	return musics[index], nil
}

func (s *SearchService) searchByEvent(eventID int, now int64) (*masterdata.Music, error) {
	musicInfo, err := s.source.GetMusicByEventID(eventID)
	if err != nil {
		return nil, err
	}
	return ensureAccessibleMusic(musicInfo, now, eventID, s.allowUnreleased)
}

func (s *SearchService) searchByBan(info *QueryInfo, now int64) (*masterdata.Music, error) {
	if fallback := strings.TrimSpace(info.Keyword); fallback != "" {
		resolved, err := s.resolveTitle(fallback)
		if err == nil && resolved != nil {
			return resolved, nil
		}
		if err != nil && isMusicAmbiguousError(err) {
			return nil, err
		}
	}
	events := s.source.GetBanEvents(info.BanCharID)
	if len(events) == 0 {
		return nil, fmt.Errorf("no ban events found for character %d", info.BanCharID)
	}
	if info.BanSeq < 1 || info.BanSeq > len(events) {
		return nil, fmt.Errorf("ban event index out of range: %d", info.BanSeq)
	}
	return s.searchByEvent(events[info.BanSeq-1].ID, now)
}

func (s *SearchService) searchByTitleOrChart(info *QueryInfo, now int64) (*masterdata.Music, error) {
	if info.MusicID != 0 {
		if musicInfo, err := s.source.GetMusicByID(info.MusicID); err == nil && musicInfo != nil {
			return ensureAccessibleMusic(musicInfo, now, info.MusicID, s.allowUnreleased)
		}
	}
	keyword := strings.TrimSpace(info.Keyword)
	if keyword != "" {
		return s.resolveTitle(keyword)
	}
	if info.MusicID != 0 {
		return nil, fmt.Errorf("music not found: %d", info.MusicID)
	}
	return nil, fmt.Errorf("music title query is empty")
}

func (s *SearchService) resolveTitle(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music title query is empty")
	}
	if s.titleResolver != nil {
		return s.titleResolver(query)
	}
	musicInfo, err := s.source.SearchMusic(query)
	if err != nil {
		return nil, err
	}
	return ensureAccessibleMusic(musicInfo, currentMusicVisibilityTime(), query, s.allowUnreleased)
}
