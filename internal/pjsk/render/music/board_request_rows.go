package music

import (
	"fmt"
	"slices"
	"sort"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (c *Controller) buildMusicBoardRows(region renderregion.Value, source DataSource, builder *Builder, query musicBoardResolvedQuery) ([]musicBoardRow, error) {
	metaMap := c.loadMusicBoardMetaMap(region.String())
	if len(metaMap) == 0 {
		return nil, fmt.Errorf("music board request has no items")
	}

	sortedSkills := resolveMusicBoardSkills(query)
	rows := make([]musicBoardRow, 0, 256)
	for musicID, metas := range metaMap {
		musicInfo, err := source.GetMusicByID(musicID)
		if err != nil || musicInfo == nil {
			continue
		}
		for _, meta := range metas {
			if row, ok := buildMusicBoardMetaRow(builder, region, musicInfo, musicID, meta, query, sortedSkills); ok {
				rows = append(rows, row)
			}
		}
	}

	sortMusicBoardRows(rows, query.Target, query.LiveType, query.Ascend, query.Target == "time")
	if query.Target == "time" {
		rows = rankedMusicBoardRows(rows)
	}
	return rows, nil
}

func resolveMusicBoardSkills(query musicBoardResolvedQuery) []float64 {
	sortedSkills := slices.Clone(query.Skills)
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
		if len(sortedSkills) > 0 {
			avg /= float64(len(sortedSkills))
		}
		for i := range sortedSkills {
			sortedSkills[i] = avg
		}
	}
	return sortedSkills
}

func buildMusicBoardMetaRow(builder *Builder, region renderregion.Value, musicInfo *masterdata.Music, musicID int, meta drawing.MusicMetaInfo, query musicBoardResolvedQuery, sortedSkills []float64) (musicBoardRow, bool) {
	level := builder.GetDifficultyLevel(musicID, meta.Difficulty)
	if level <= 0 {
		return musicBoardRow{}, false
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

	row := musicBoardRow{
		MusicID:        musicID,
		Difficulty:     normalizeDifficulty(meta.Difficulty),
		Level:          level,
		MusicTitle:     builder.buildDisplayMusicTitle(musicInfo, region),
		MusicCoverPath: builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region),
		EventRate:      meta.EventRate,
		MusicTime:      meta.MusicTime,
		Tps:            tps,
	}
	populateMusicBoardLiveMetrics(&row, "solo", soloScore, musicBoardSkillAccount(soloSkill, soloScore), query.Power, query.DeckBonus, query.PlayInterval)
	populateMusicBoardLiveMetrics(&row, "auto", autoScore, musicBoardSkillAccount(autoSkill, autoScore), query.Power, query.DeckBonus, query.PlayInterval)
	populateMusicBoardLiveMetrics(&row, "multi", multiScore, musicBoardSkillAccount(multiSkill, multiScore), query.Power, query.DeckBonus, query.PlayInterval)
	return row, true
}

func rankedMusicBoardRows(rows []musicBoardRow) []musicBoardRow {
	filtered := make([]musicBoardRow, 0, len(rows))
	for _, row := range rows {
		if row.Rank > 0 {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
