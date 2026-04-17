package music

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
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
		musicInfo, err := s.source.GetMusicByID(info.Value)
		if err == nil && musicInfo != nil && isMusicVisibleAt(musicInfo, now) {
			return musicInfo, nil
		}
		if info.AllowTitleFallback {
			if fallback := strings.TrimSpace(info.Keyword); fallback != "" {
				if resolved, fallbackErr := s.resolveTitle(fallback); fallbackErr == nil && resolved != nil {
					return resolved, nil
				}
			}
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("music not found: %d", info.Value)

	case QueryTypeSeq:
		musics := visibleMusicsSortedByPublishedAt(s.source, now)
		if len(musics) == 0 {
			return nil, fmt.Errorf("no music data available")
		}

		index := info.Value
		if index < 0 {
			index = len(musics) + index
		} else {
			index--
		}
		if index < 0 || index >= len(musics) {
			return nil, fmt.Errorf("music index out of range: %d", info.Value)
		}
		return musics[index], nil

	case QueryTypeEvent:
		musicInfo, err := s.source.GetMusicByEventID(info.Value)
		if err != nil {
			return nil, err
		}
		return ensureVisibleMusic(musicInfo, now, info.Value)

	case QueryTypeBan:
		events := s.source.GetBanEvents(info.BanCharID)
		if len(events) == 0 {
			return nil, fmt.Errorf("no ban events found for character %d", info.BanCharID)
		}
		if info.BanSeq < 1 || info.BanSeq > len(events) {
			return nil, fmt.Errorf("ban event index out of range: %d", info.BanSeq)
		}
		musicInfo, err := s.source.GetMusicByEventID(events[info.BanSeq-1].ID)
		if err != nil {
			return nil, err
		}
		return ensureVisibleMusic(musicInfo, now, events[info.BanSeq-1].ID)

	case QueryTypeTitle, QueryTypeChart:
		if info.MusicID != 0 {
			if musicInfo, err := s.source.GetMusicByID(info.MusicID); err == nil && musicInfo != nil {
				return musicInfo, nil
			}
		}
		keyword := strings.TrimSpace(info.Keyword)
		if keyword == "" {
			if info.MusicID != 0 {
				return nil, fmt.Errorf("music not found: %d", info.MusicID)
			}
			return nil, fmt.Errorf("music title query is empty")
		}
		return s.resolveTitle(keyword)

	default:
		return nil, fmt.Errorf("unsupported music query type: %d", info.Type)
	}
}

func (s *SearchService) resolveTitle(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music title query is empty")
	}
	if s.titleResolver != nil {
		return s.titleResolver(query)
	}
	return s.source.SearchMusic(query)
}
