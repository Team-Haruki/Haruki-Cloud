package music

import (
	"fmt"
	"slices"
	"sort"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) buildMusicBoardRows(region renderregion.Value, source DataSource, builder *Builder, query musicBoardResolvedQuery) ([]musicBoardRow, error) {
	metaMap := c.loadMusicBoardMetaMap(region.String())
	if len(metaMap) == 0 {
		return nil, fmt.Errorf("music board request has no items")
	}

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
