package music

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
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

	specMap := make(map[string]struct{}, len(specs))
	specRankMap := make(map[int]struct{}, len(specs))
	specRows := make([]musicBoardRow, 0, len(specs))
	for _, spec := range specs {
		key := musicBoardKey(spec.MusicID, spec.Difficulty)
		specMap[key] = struct{}{}
		for _, row := range rows {
			if row.MusicID != spec.MusicID || row.Difficulty != spec.Difficulty {
				continue
			}
			specRows = append(specRows, row)
			specRankMap[row.Rank] = struct{}{}
			break
		}
	}

	filtered := make([]musicBoardRow, 0, len(rows))
	for _, row := range rows {
		if _, exists := specRankMap[row.Rank]; exists {
			continue
		}
		if len(normalized.DiffFilter) > 0 && !common.ContainsString(normalized.DiffFilter, row.Difficulty) {
			continue
		}
		if !matchesMusicBoardLevelFilter(row.Level, normalized.LevelFilter) {
			continue
		}
		filtered = append(filtered, row)
	}

	showRows := append([]musicBoardRow(nil), specRows...)
	remainingSize := musicBoardPageSize - len(showRows)
	totalPage := 1
	if remainingSize <= 0 {
		remainingSize = 0
	}
	if len(filtered) > 0 && remainingSize > 0 {
		totalPage = int(math.Ceil(float64(len(filtered)) / float64(remainingSize)))
		if normalized.Page < 1 || normalized.Page > totalPage {
			return nil, fmt.Errorf("页数错误，当前筛选结果仅有%d页", totalPage)
		}
		start := (normalized.Page - 1) * remainingSize
		end := start + remainingSize
		if end > len(filtered) {
			end = len(filtered)
		}
		showRows = append(showRows, filtered[start:end]...)
	} else if len(filtered) == 0 && len(showRows) == 0 {
		return nil, fmt.Errorf("筛选后的歌曲数为零")
	}

	sort.Slice(showRows, func(i, j int) bool {
		return showRows[i].Rank < showRows[j].Rank
	})

	specMidDiffs := make([][]interface{}, 0, len(specs))
	for _, spec := range specs {
		specMidDiffs = append(specMidDiffs, []interface{}{spec.MusicID, spec.Difficulty})
	}

	items := make([]drawing.MusicBoardItem, 0, len(showRows))
	for _, row := range showRows {
		items = append(items, drawing.MusicBoardItem{
			Rank:                 row.Rank,
			MusicID:              row.MusicID,
			Difficulty:           row.Difficulty,
			Level:                row.Level,
			MusicTitle:           row.MusicTitle,
			MusicCoverPath:       row.MusicCoverPath,
			LiveTypePt:           selectMusicBoardLiveValue(row, normalized.LiveType, "pt"),
			LiveTypeRealScore:    selectMusicBoardLiveValue(row, normalized.LiveType, "real_score"),
			LiveTypeScore:        selectMusicBoardLiveValue(row, normalized.LiveType, "score"),
			LiveTypeSkillAccount: selectMusicBoardLiveValue(row, normalized.LiveType, "skill_account"),
			LiveTypePtPerHour:    selectMusicBoardLiveValue(row, normalized.LiveType, "pt_per_hour"),
			PlayCountPerHour:     row.PlayCountPerHour,
			EventRate:            row.EventRate,
			MusicTime:            row.MusicTime,
			Tps:                  row.Tps,
		})
	}

	titleText, description := buildMusicBoardTexts(normalized, totalPage)
	return &drawing.MusicBoardRequest{
		LiveType:     normalized.LiveType,
		Target:       normalized.Target,
		Ascend:       normalized.Ascend,
		Page:         normalized.Page,
		TotalPage:    totalPage,
		TitleText:    titleText,
		Items:        items,
		SpecMidDiffs: specMidDiffs,
		Description:  description,
	}, nil
}

func normalizeMusicBoardQuery(query BoardQuery) musicBoardResolvedQuery {
	liveType := strings.ToLower(strings.TrimSpace(query.LiveType))
	if _, ok := musicBoardLiveTypes[liveType]; !ok {
		liveType = "solo"
	}

	target := strings.ToLower(strings.TrimSpace(query.Target))
	if _, ok := musicBoardTargets[target]; !ok {
		switch liveType {
		case "multi":
			target = "pt/time"
		default:
			target = "score"
		}
	}

	strategy := strings.ToLower(strings.TrimSpace(query.SkillStrategy))
	if _, ok := musicBoardStrategies[strategy]; !ok {
		switch liveType {
		case "solo":
			strategy = "max"
		default:
			strategy = "avg"
		}
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}

	power := query.Power
	if power <= 0 {
		power = musicBoardDefaultPower
	}

	deckBonus := query.DeckBonus
	if deckBonus <= 0 {
		deckBonus = musicBoardDefaultDeckBonus
	}

	playInterval := query.PlayInterval
	if playInterval <= 0 {
		switch liveType {
		case "multi":
			playInterval = musicBoardDefaultMultiInterval
		default:
			playInterval = musicBoardDefaultSoloInterval
		}
	}

	skills := normalizeMusicBoardSkills(query.Skills, liveType)

	diffFilter := make([]string, 0, len(query.DiffFilter))
	for _, diff := range query.DiffFilter {
		normalized := normalizeDifficulty(diff)
		if normalized == "" {
			continue
		}
		if common.ContainsString(diffFilter, normalized) {
			continue
		}
		diffFilter = append(diffFilter, normalized)
	}

	specQueries := make([]string, 0, len(query.SpecQueries))
	for _, raw := range query.SpecQueries {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		specQueries = append(specQueries, text)
	}

	return musicBoardResolvedQuery{
		LiveType:      liveType,
		Target:        target,
		Ascend:        query.Ascend,
		Page:          page,
		SkillStrategy: strategy,
		Skills:        skills,
		Power:         power,
		DeckBonus:     deckBonus,
		PlayInterval:  playInterval,
		DiffFilter:    diffFilter,
		LevelFilter:   strings.TrimSpace(query.LevelFilter),
		SpecQueries:   specQueries,
	}
}

func normalizeMusicBoardSkills(skills []float64, liveType string) []float64 {
	clean := make([]float64, 0, 5)
	for _, skill := range skills {
		if skill <= 0 {
			continue
		}
		clean = append(clean, skill)
	}

	switch {
	case liveType == "multi" && len(clean) == 1:
		return []float64{clean[0], clean[0], clean[0], clean[0], clean[0]}
	case len(clean) >= 5:
		return append([]float64(nil), clean[:5]...)
	case liveType == "multi":
		return []float64{
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
			musicBoardDefaultMultiSkill,
		}
	default:
		return []float64{
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
			musicBoardDefaultSoloSkill,
		}
	}
}

func (c *Controller) buildMusicBoardRows(region renderregion.Value, source DataSource, builder *Builder, query musicBoardResolvedQuery) ([]musicBoardRow, error) {
	metaMap := c.loadMusicBoardMetaMap(region.String())
	if len(metaMap) == 0 {
		return nil, fmt.Errorf("music board request has no items")
	}

	sortedSkills := append([]float64(nil), query.Skills...)
	switch query.SkillStrategy {
	case "max":
		sort.Slice(sortedSkills, func(i, j int) bool { return sortedSkills[i] > sortedSkills[j] })
	case "min":
		sort.Slice(sortedSkills, func(i, j int) bool { return sortedSkills[i] < sortedSkills[j] })
	case "avg":
		avg := 0.0
		for _, skill := range sortedSkills {
			avg += skill
		}
		avg /= float64(len(sortedSkills))
		for i := range sortedSkills {
			sortedSkills[i] = avg
		}
	}

	rows := make([]musicBoardRow, 0, 256)
	for musicID, metas := range metaMap {
		musicInfo, err := source.GetMusicByID(musicID)
		if err != nil || musicInfo == nil {
			continue
		}
		title := builder.buildDisplayMusicTitle(musicInfo, region)
		coverPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)

		for _, meta := range metas {
			level := builder.GetDifficultyLevel(musicID, meta.Difficulty)
			if level <= 0 {
				continue
			}

			tps := 0.0
			if meta.MusicTime > 0 {
				tps = float64(meta.TapCount) / meta.MusicTime
			}

			soloSkill := weightedMusicBoardSkill(meta.SkillScoreSolo, sortedSkills, query.Skills[0])
			autoSkill := weightedMusicBoardSkill(meta.SkillScoreAuto, sortedSkills, query.Skills[0])
			multiSkill := weightedMusicBoardSkill(meta.SkillScoreMulti, sortedSkills, query.Skills[0])

			soloScore := meta.BaseScore + soloSkill
			autoScore := meta.BaseScoreAuto + autoSkill
			multiScore := meta.BaseScore + multiSkill + meta.FeverScore*0.5 + 0.01875

			soloSkillAccount := musicBoardSkillAccount(soloSkill, soloScore)
			autoSkillAccount := musicBoardSkillAccount(autoSkill, autoScore)
			multiSkillAccount := musicBoardSkillAccount(multiSkill, multiScore)

			row := musicBoardRow{
				MusicID:        musicID,
				Difficulty:     normalizeDifficulty(meta.Difficulty),
				Level:          level,
				MusicTitle:     title,
				MusicCoverPath: coverPath,
				EventRate:      meta.EventRate,
				MusicTime:      meta.MusicTime,
				Tps:            tps,
			}
			populateMusicBoardLiveMetrics(&row, "solo", soloScore, soloSkillAccount, query.Power, query.DeckBonus, query.PlayInterval)
			populateMusicBoardLiveMetrics(&row, "auto", autoScore, autoSkillAccount, query.Power, query.DeckBonus, query.PlayInterval)
			populateMusicBoardLiveMetrics(&row, "multi", multiScore, multiSkillAccount, query.Power, query.DeckBonus, query.PlayInterval)
			rows = append(rows, row)
		}
	}

	sortMusicBoardRows(rows, query.Target, query.LiveType, query.Ascend, query.Target == "time")
	if query.Target == "time" {
		filtered := make([]musicBoardRow, 0, len(rows))
		for _, row := range rows {
			if row.Rank > 0 {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	return rows, nil
}
