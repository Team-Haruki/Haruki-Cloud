package music

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type SearchService struct {
	source        DataSource
	parser        *Parser
	titleResolver func(string) (*masterdata.Music, error)
}

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

	switch info.Type {
	case QueryTypeID:
		return s.source.GetMusicByID(info.Value)

	case QueryTypeSeq:
		musics := append([]*masterdata.Music(nil), s.source.GetMusics()...)
		if len(musics) == 0 {
			return nil, fmt.Errorf("no music data available")
		}
		sort.Slice(musics, func(i, j int) bool {
			if musics[i].PublishedAt == musics[j].PublishedAt {
				return musics[i].ID < musics[j].ID
			}
			return musics[i].PublishedAt < musics[j].PublishedAt
		})

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
		return s.source.GetMusicByEventID(info.Value)

	case QueryTypeBan:
		events := s.source.GetBanEvents(info.BanCharID)
		if len(events) == 0 {
			return nil, fmt.Errorf("no ban events found for character %d", info.BanCharID)
		}
		if info.BanSeq < 1 || info.BanSeq > len(events) {
			return nil, fmt.Errorf("ban event index out of range: %d", info.BanSeq)
		}
		return s.source.GetMusicByEventID(events[info.BanSeq-1].ID)

	case QueryTypeTitle, QueryTypeChart:
		keyword := strings.TrimSpace(info.Keyword)
		if keyword == "" && info.MusicID != 0 {
			return s.source.GetMusicByID(info.MusicID)
		}
		if s.titleResolver != nil {
			return s.titleResolver(keyword)
		}
		return s.source.SearchMusic(keyword)

	default:
		return nil, fmt.Errorf("unsupported music query type: %d", info.Type)
	}
}
