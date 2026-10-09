package music

import (
	"math"
	"slices"
	"strconv"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

var (
	musicDetailLeaderboardLiveTypeOrder = []string{"solo", "multi", "auto"}
	musicDetailLeaderboardTargetOrder   = []string{"score", "pt", pointsPerTimeMetric}

	musicDetailLeaderboardTargets = map[string]i18n.Message{
		"score":             i18n.M("render_music.leaderboard.target.score"),
		"pt":                i18n.M("render_music.leaderboard.target.pt"),
		pointsPerTimeMetric: i18n.M("render_music.leaderboard.target.pt_time"),
	}
	musicDetailLeaderboardSkills = map[string][]float64{
		"solo":  {musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill},
		"auto":  {musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill, musicBoardDefaultSoloSkill},
		"multi": {musicBoardDefaultMultiSkill, musicBoardDefaultMultiSkill, musicBoardDefaultMultiSkill, musicBoardDefaultMultiSkill, musicBoardDefaultMultiSkill},
	}
	musicDetailLeaderboardIntervals = map[string]float64{
		"solo":  musicBoardDefaultSoloInterval,
		"auto":  musicBoardDefaultSoloInterval,
		"multi": musicBoardDefaultMultiInterval,
	}
)

func (c *Controller) enrichMusicDetailRequest(req *drawing.MusicDetailRequest, region renderregion.Value, source DataSource, builder *Builder, musicInfo *masterdata.Music, preferredDifficulty string) {
	if c == nil || req == nil || musicInfo == nil {
		return
	}

	if length := c.resolveMusicDetailLength(region.String(), musicInfo.ID); length != nil {
		req.Length = length
	}
	if bpm := c.resolveMusicDetailBPM(region, musicInfo.ID, preferredDifficulty); bpm != nil {
		req.Bpm = bpm
	}

	matrix, total := c.resolveMusicDetailLeaderboard(region, source, builder, musicInfo.ID)
	if len(matrix) == 0 || total <= 0 {
		return
	}

	req.LeaderboardMatrix = matrix
	req.LeaderboardLiveTypes = musicDetailLiveTypeLabels(musicDetailLeaderboardLiveTypeOrder)
	req.LeaderboardTargets = cloneMusicDetailLabels(musicDetailLeaderboardTargets, musicDetailLeaderboardTargetOrder)
	req.LeaderboardMusicNum = intPtr(total)
}

func (c *Controller) resolveMusicDetailBPM(region renderregion.Value, musicID int, preferredDifficulty string) *int {
	if c == nil || musicID <= 0 {
		return nil
	}

	for _, difficulty := range buildBPMDifficultyCandidates(preferredDifficulty) {
		parsed, found, err := c.loadChartBPM(c.contextOrBackground(), region.String(), musicID, difficulty)
		if !found || err != nil || parsed == nil || parsed.MainBPM <= 0 {
			continue
		}
		bpm := int(math.Round(parsed.MainBPM))
		if bpm <= 0 {
			continue
		}
		return &bpm
	}
	return nil
}

func (c *Controller) resolveMusicDetailLength(region string, musicID int) *string {
	metas := c.resolveAllMusicMetas(region, musicID)
	if len(metas) == 0 {
		return nil
	}

	maxSeconds := 0.0
	for _, item := range metas {
		if item.MusicTime > maxSeconds {
			maxSeconds = item.MusicTime
		}
	}
	if maxSeconds <= 0 {
		return nil
	}

	return new(formatMusicDetailLength(maxSeconds))
}

func formatMusicDetailLength(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	return i18n.T("render_music.detail.length", i18n.Data{
		"Seconds":  strconv.FormatFloat(seconds, 'f', 1, 64),
		"Duration": i18n.FormatDuration(time.Duration(seconds * float64(time.Second))),
	})
}

func (c *Controller) resolveMusicDetailLeaderboard(region renderregion.Value, source DataSource, builder *Builder, musicID int) ([][]*drawing.LeaderboardInfo, int) {
	if c == nil || source == nil || builder == nil || musicID <= 0 {
		return nil, 0
	}

	matrix := make([][]*drawing.LeaderboardInfo, 0, len(musicDetailLeaderboardLiveTypeOrder))
	totalSongs := 0

	for _, liveType := range musicDetailLeaderboardLiveTypeOrder {
		skills := slices.Clone(musicDetailLeaderboardSkills[liveType])
		rows, err := c.buildMusicBoardRows(region, source, builder, musicBoardResolvedQuery{
			LiveType:      liveType,
			Target:        "score",
			Page:          1,
			SkillStrategy: "avg",
			Skills:        skills,
			Power:         musicBoardDefaultPower,
			DeckBonus:     musicBoardDefaultDeckBonus,
			PlayInterval:  musicDetailLeaderboardIntervals[liveType],
		})
		if err != nil || len(rows) == 0 {
			return nil, 0
		}

		rowMatrix := make([]*drawing.LeaderboardInfo, 0, len(musicDetailLeaderboardTargetOrder))
		for _, target := range musicDetailLeaderboardTargetOrder {
			sortedRows := slices.Clone(rows)
			sortMusicBoardRows(sortedRows, target, liveType, false, true)

			info, rankedSongs := findMusicDetailLeaderboardInfo(sortedRows, musicID, liveType, target)
			if rankedSongs > totalSongs {
				totalSongs = rankedSongs
			}
			rowMatrix = append(rowMatrix, info)
		}
		matrix = append(matrix, rowMatrix)
	}

	return matrix, totalSongs
}

func findMusicDetailLeaderboardInfo(rows []musicBoardRow, musicID int, liveType, target string) (*drawing.LeaderboardInfo, int) {
	rankedSongs := 0
	for _, row := range rows {
		if row.Rank > 0 {
			rankedSongs++
		}
	}
	for _, row := range rows {
		if row.MusicID != musicID || row.Rank <= 0 {
			continue
		}
		return &drawing.LeaderboardInfo{
			Rank:  row.Rank,
			Diff:  row.Difficulty,
			Value: formatMusicDetailLeaderboardValue(row, liveType, target),
		}, rankedSongs
	}
	return nil, rankedSongs
}

func formatMusicDetailLeaderboardValue(row musicBoardRow, liveType, target string) string {
	switch target {
	case "score":
		score := derefMusicBoardFloat(selectMusicBoardLiveValue(row, liveType, "score"))
		return i18n.Percent(score * 100)
	case "pt":
		pt := derefMusicBoardFloat(selectMusicBoardLiveValue(row, liveType, "pt"))
		return strconv.Itoa(int(math.Round(pt)))
	case pointsPerTimeMetric:
		ptPerHour := derefMusicBoardFloat(selectMusicBoardLiveValue(row, liveType, pointsPerTimeMetric))
		return i18n.T("render_music.leaderboard.pt_per_hour", i18n.Data{"Value": i18n.Wan(ptPerHour)})
	default:
		return "-"
	}
}

func cloneMusicDetailLabels(input map[string]i18n.Message, order []string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for _, key := range order {
		if value, ok := input[key]; ok {
			result[key] = value.String()
		}
	}
	return result
}

// musicDetailLiveTypeLabels names the leaderboard rows with the short live
// type labels; the row header is too narrow for "单人 Live".
func musicDetailLiveTypeLabels(order []string) map[string]string {
	result := make(map[string]string, len(order))
	for _, key := range order {
		result[key] = common.LiveShortLabel(key)
	}
	return result
}

func intPtr(value int) *int {
	return &value
}
