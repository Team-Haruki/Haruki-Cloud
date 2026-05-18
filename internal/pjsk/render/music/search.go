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
		musicInfo, err := s.source.GetMusicByID(info.Value)
		if err == nil && musicInfo != nil {
			if isMusicAccessibleAt(musicInfo, now, s.allowUnreleased) {
				return musicInfo, nil
			}
			return nil, releasecheck.New(releasecheck.KindMusic, "", info.Value)
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
		// Sequence lookup should stay anchored to songs that are already
		// released in the target region, even when title/id lookup is allowed
		// to peek at unreleased data.
		musics := accessibleMusicsSortedByPublishedAt(s.source, now, false)
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
		return ensureAccessibleMusic(musicInfo, now, info.Value, s.allowUnreleased)

	case QueryTypeBan:
		if fallback := strings.TrimSpace(info.Keyword); fallback != "" {
			if resolved, fallbackErr := s.resolveTitle(fallback); fallbackErr == nil && resolved != nil {
				return resolved, nil
			} else if fallbackErr != nil && isMusicAmbiguousError(fallbackErr) {
				return nil, fallbackErr
			}
		}
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
		return ensureAccessibleMusic(musicInfo, now, events[info.BanSeq-1].ID, s.allowUnreleased)

	case QueryTypeTitle, QueryTypeChart:
		if info.MusicID != 0 {
			if musicInfo, err := s.source.GetMusicByID(info.MusicID); err == nil && musicInfo != nil {
				return ensureAccessibleMusic(musicInfo, now, info.MusicID, s.allowUnreleased)
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
	musicInfo, err := s.source.SearchMusic(query)
	if err != nil {
		return nil, err
	}
	return ensureAccessibleMusic(musicInfo, currentMusicVisibilityTime(), query, s.allowUnreleased)
}
