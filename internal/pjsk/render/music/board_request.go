package music

import (
	"fmt"
	"math"
	"slices"
	"sort"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/common"
)

const (
	musicBoardPageSize             = 50
	musicBoardDefaultPower         = 300000
	musicBoardDefaultDeckBonus     = 400.0
	musicBoardDefaultSoloSkill     = 1.2
	musicBoardDefaultMultiSkill    = 2.0
	musicBoardDefaultSoloInterval  = 28.0
	musicBoardDefaultMultiInterval = 45.2
)

var (
	musicBoardLiveTypes = map[string]struct{}{
		"solo":  {},
		"auto":  {},
		"multi": {},
	}
	musicBoardTargets = map[string]struct{}{
		"score":   {},
		"pt":      {},
		"pt/time": {},
		"tps":     {},
		"time":    {},
	}
	musicBoardStrategies = map[string]struct{}{
		"max": {},
		"min": {},
		"avg": {},
	}
)

type musicBoardRow struct {
	Rank              int
	MusicID           int
	Difficulty        string
	Level             int
	MusicTitle        string
	MusicCoverPath    string
	SoloPt            *float64
	SoloRealScore     *float64
	SoloScore         *float64
	SoloSkillAccount  *float64
	SoloPtPerHour     *float64
	AutoPt            *float64
	AutoRealScore     *float64
	AutoScore         *float64
	AutoSkillAccount  *float64
	AutoPtPerHour     *float64
	MultiPt           *float64
	MultiRealScore    *float64
	MultiScore        *float64
	MultiSkillAccount *float64
	MultiPtPerHour    *float64
	PlayCountPerHour  *float64
	EventRate         float64
	MusicTime         float64
	Tps               float64
}

type musicBoardResolvedQuery struct {
	LiveType      string
	Target        string
	Ascend        bool
	Page          int
	SkillStrategy string
	Skills        []float64
	Power         int
	DeckBonus     float64
	PlayInterval  float64
	DiffFilter    []string
	LevelFilter   string
	SpecQueries   []string
}

type musicBoardSpec struct {
	MusicID    int
	Difficulty string
}

func (c *Controller) ResolveMusicBoardRequest(region string, query BoardQuery) (*drawing.MusicBoardRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}

	resolvedRegion, source, builder, err := c.resolveBuilder(region)
	if err != nil {
		return nil, err
	}

	normalized := normalizeMusicBoardQuery(query)
	rows, err := c.buildMusicBoardRows(resolvedRegion, source, builder, normalized)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("music board request has no items")
	}

	specs, err := c.resolveMusicBoardSpecs(source, rows, normalized.SpecQueries)
	if err != nil {
		return nil, err
	}
	if len(specs) >= musicBoardPageSize {
		return nil, fmt.Errorf("最多只能关注%d首歌曲", musicBoardPageSize-1)
	}

	specRows, specRankMap := selectMusicBoardSpecRows(rows, specs)
	filtered := filterMusicBoardRows(rows, specRankMap, normalized)
	showRows, totalPage, err := paginateMusicBoardRows(specRows, filtered, normalized.Page)
	if err != nil {
		return nil, err
	}

	titleText, description := buildMusicBoardTexts(normalized, totalPage)
	return &drawing.MusicBoardRequest{
		LiveType:     normalized.LiveType,
		Target:       normalized.Target,
		Ascend:       normalized.Ascend,
		Page:         normalized.Page,
		TotalPage:    totalPage,
		TitleText:    titleText,
		Items:        musicBoardDrawingItems(showRows, normalized.LiveType),
		SpecMidDiffs: musicBoardSpecMidDiffs(specs),
		Description:  description,
	}, nil
}

func selectMusicBoardSpecRows(rows []musicBoardRow, specs []musicBoardSpec) ([]musicBoardRow, map[int]struct{}) {
	specRows := make([]musicBoardRow, 0, len(specs))
	specRankMap := make(map[int]struct{}, len(specs))
	for _, spec := range specs {
		for _, row := range rows {
			if row.MusicID == spec.MusicID && row.Difficulty == spec.Difficulty {
				specRows = append(specRows, row)
				specRankMap[row.Rank] = struct{}{}
				break
			}
		}
	}
	return specRows, specRankMap
}

func filterMusicBoardRows(rows []musicBoardRow, excludedRanks map[int]struct{}, query musicBoardResolvedQuery) []musicBoardRow {
	filtered := make([]musicBoardRow, 0, len(rows))
	for _, row := range rows {
		if _, exists := excludedRanks[row.Rank]; exists {
			continue
		}
		if len(query.DiffFilter) > 0 && !common.ContainsString(query.DiffFilter, row.Difficulty) {
			continue
		}
		if matchesMusicBoardLevelFilter(row.Level, query.LevelFilter) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func paginateMusicBoardRows(specRows, filtered []musicBoardRow, page int) ([]musicBoardRow, int, error) {
	showRows := slices.Clone(specRows)
	remainingSize := max(0, musicBoardPageSize-len(showRows))
	totalPage := 1
	if len(filtered) > 0 && remainingSize > 0 {
		totalPage = int(math.Ceil(float64(len(filtered)) / float64(remainingSize)))
		if page < 1 || page > totalPage {
			return nil, totalPage, fmt.Errorf("页数错误，当前筛选结果仅有%d页", totalPage)
		}
		start := (page - 1) * remainingSize
		end := min(start+remainingSize, len(filtered))
		showRows = append(showRows, filtered[start:end]...)
	} else if len(showRows) == 0 {
		return nil, totalPage, fmt.Errorf("筛选后的歌曲数为零")
	}
	sort.Slice(showRows, func(i, j int) bool { return showRows[i].Rank < showRows[j].Rank })
	return showRows, totalPage, nil
}

func musicBoardSpecMidDiffs(specs []musicBoardSpec) [][]any {
	result := make([][]any, 0, len(specs))
	for _, spec := range specs {
		result = append(result, []any{spec.MusicID, spec.Difficulty})
	}
	return result
}

func musicBoardDrawingItems(rows []musicBoardRow, liveType string) []drawing.MusicBoardItem {
	items := make([]drawing.MusicBoardItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, drawing.MusicBoardItem{
			Rank:                 row.Rank,
			MusicID:              row.MusicID,
			Difficulty:           row.Difficulty,
			Level:                row.Level,
			MusicTitle:           row.MusicTitle,
			MusicCoverPath:       row.MusicCoverPath,
			LiveTypePt:           selectMusicBoardLiveValue(row, liveType, "pt"),
			LiveTypeRealScore:    selectMusicBoardLiveValue(row, liveType, "real_score"),
			LiveTypeScore:        selectMusicBoardLiveValue(row, liveType, "score"),
			LiveTypeSkillAccount: selectMusicBoardLiveValue(row, liveType, "skill_account"),
			LiveTypePtPerHour:    selectMusicBoardLiveValue(row, liveType, "pt_per_hour"),
			PlayCountPerHour:     row.PlayCountPerHour,
			EventRate:            row.EventRate,
			MusicTime:            row.MusicTime,
			Tps:                  row.Tps,
		})
	}
	return items
}
