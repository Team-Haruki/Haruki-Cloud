package handler

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/render/music"
	sekaiutils "haruki-cloud/internal/pjsk/sekai"
)

func executeMusic(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App == nil || rc.App.Music == nil {
		return nil, fmt.Errorf("music service unavailable: music controller is not configured")
	}
	musicCtrl := rc.App.Music.WithContext(rc.Ctx)
	if rc.App.Aliases != nil {
		musicCtrl.SetAliasResolver(rc.App.Aliases)
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "music-detail":
		q := music.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = musicCtrl.RenderMusicDetail(q)
	case "music-list":
		_, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
		if suiteErr != nil {
			return nil, suiteErr
		}
		if suiteSnapshot != nil {
			musicCtrl = musicCtrl.WithSnapshot(suiteSnapshot)
		}
		q := music.ListQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if strings.TrimSpace(q.Keyword) == "" {
			q.Keyword = strings.TrimSpace(rc.Cmd.Query)
		}
		if suiteSnapshot != nil {
			q.DetailedProfile = suiteSnapshot.DetailedProfile(rc.Region)
		} else {
			q.DetailedProfile = rc.GetDetailedProfile()
		}
		data, err = musicCtrl.RenderMusicList(q)
	case "music-chart":
		q := music.ChartQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = musicCtrl.RenderMusicChart(q)
	case "music-progress":
		_, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
		if suiteErr != nil {
			return nil, suiteErr
		}
		q := music.ProgressQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if suiteSnapshot != nil {
			data, err = musicCtrl.RenderMusicProgressFromSnapshot(q, suiteSnapshot, suiteSnapshot.ProfileCard(rc.Region))
		} else {
			data, err = musicCtrl.RenderMusicProgressFromSnapshot(q, nil, rc.GetProfileCard())
		}
	case "music-rewards":
		data, err = renderMusicRewards(rc)
	case "music-note-count":
		q := music.NoteCountQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		matches, resolveErr := musicCtrl.FindMusicChartsByNoteCount(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if len(matches) == 1 {
			data, err = renderSingleMusicLookupChart(musicCtrl, rc.Cmd.Region, matches[0].Music.ID, matches[0].Difficulty)
			break
		}
		return renderNoteCountLookupListMessages(rc, musicCtrl, q, matches)
	case "music-cover":
		q := music.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := musicCtrl.ResolveMusicCover(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(rc.Ctx, result.JacketPath, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		text := fmt.Sprintf("【%d】%s", result.Music.ID, result.Music.Title)
		return append(image, onebot11.Text(text)), nil
	case "music-bpm":
		q := music.BPMQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if q.BPM <= 0 {
			if value, parseErr := strconv.ParseFloat(strings.TrimSpace(rc.Cmd.Query), 64); parseErr == nil {
				q.BPM = value
			}
		}
		matches, resolveErr := musicCtrl.FindMusicChartsByBPM(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if len(matches) == 1 {
			data, err = renderSingleMusicLookupChart(musicCtrl, rc.Cmd.Region, matches[0].Music.ID, matches[0].Difficulty)
			break
		}
		return renderBPMLookupListMessages(rc, musicCtrl, q, matches)
	default:
		return nil, unsupportedModeError("music", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func renderSingleMusicLookupChart(musicCtrl *music.Controller, region string, musicID int, difficulty string) ([]byte, error) {
	return musicCtrl.RenderMusicChart(music.ChartQuery{
		Query:      fmt.Sprintf("music%d", musicID),
		Region:     region,
		Difficulty: difficulty,
	})
}

func renderNoteCountLookupListMessages(rc *RequestContext, musicCtrl *music.Controller, query music.NoteCountQuery, matches []music.NoteCountMatch) (onebot11.Message, error) {
	items := make([]music.ListItemQuery, 0, len(matches))
	for _, item := range matches {
		if item.Music == nil {
			continue
		}
		items = append(items, music.ListItemQuery{
			MusicID:    item.Music.ID,
			Difficulty: item.Difficulty,
			Level:      item.PlayLevel,
		})
	}
	return renderMusicLookupListMessages(rc, musicCtrl, rc.Cmd.Region, "物量", strconv.Itoa(query.NoteCount), query.Difficulty, query.Difficulty, items)
}

func renderBPMLookupListMessages(rc *RequestContext, musicCtrl *music.Controller, query music.BPMQuery, matches []music.BPMMatch) (onebot11.Message, error) {
	uniqueMatches := dedupeBPMMatchesByMusic(matches)
	items := make([]music.BriefListItemQuery, 0, len(uniqueMatches))
	for _, item := range uniqueMatches {
		if item.Music == nil {
			continue
		}
		items = append(items, music.BriefListItemQuery{
			MusicID: item.Music.ID,
		})
	}
	return renderMusicBriefLookupListMessages(rc, musicCtrl, rc.Cmd.Region, "BPM", formatMusicBPM(query.BPM), items)
}

func renderMusicLookupListMessages(rc *RequestContext, musicCtrl *music.Controller, region string, prefix string, value string, requestDifficulty string, titleDifficulty string, items []music.ListItemQuery) (onebot11.Message, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}
	title := buildMusicLookupListTitle(prefix, value, titleDifficulty)
	data, err := musicCtrl.RenderMusicList(music.ListQuery{
		Items:       items,
		Difficulty:  requestDifficulty,
		Region:      region,
		Title:       &title,
		TitleShadow: true,
	})
	if err != nil {
		return nil, err
	}
	image, err := rc.ImageMessage(data)
	if err != nil {
		return nil, err
	}
	return image, nil
}

func renderMusicBriefLookupListMessages(rc *RequestContext, musicCtrl *music.Controller, region string, prefix string, value string, items []music.BriefListItemQuery) (onebot11.Message, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}
	title := buildMusicLookupListTitle(prefix, value, "")
	data, err := musicCtrl.RenderMusicBriefList(music.BriefListQuery{
		Items:       items,
		Region:      region,
		Title:       &title,
		TitleShadow: true,
	})
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func dedupeBPMMatchesByMusic(matches []music.BPMMatch) []music.BPMMatch {
	if len(matches) == 0 {
		return nil
	}
	result := make([]music.BPMMatch, 0, len(matches))
	seen := make(map[int]struct{}, len(matches))
	for _, match := range matches {
		if match.Music == nil {
			continue
		}
		if _, ok := seen[match.Music.ID]; ok {
			continue
		}
		seen[match.Music.ID] = struct{}{}
		result = append(result, match)
	}
	return result
}

func buildMusicLookupListTitle(prefix string, value string, difficulty string) string {
	title := fmt.Sprintf("%s %s 匹配结果", strings.TrimSpace(prefix), strings.TrimSpace(value))
	if diffLabel := formatMusicDifficultyLabel(difficulty); diffLabel != "" {
		title = fmt.Sprintf("%s %s %s 匹配结果", strings.TrimSpace(prefix), strings.TrimSpace(value), diffLabel)
	}
	return strings.TrimSpace(title)
}

func formatMusicDifficultyLabel(difficulty string) string {
	switch strings.ToLower(strings.TrimSpace(difficulty)) {
	case "easy":
		return "EASY"
	case "normal":
		return "NORMAL"
	case "hard":
		return "HARD"
	case "expert":
		return "EXPERT"
	case "master":
		return "MASTER"
	case "append":
		return "APPEND"
	default:
		return ""
	}
}

func formatMusicBPM(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func renderMusicRewards(rc *RequestContext) ([]byte, error) {
	musicCtrl := rc.App.Music.WithContext(rc.Ctx)
	if rc.App.Aliases != nil {
		musicCtrl.SetAliasResolver(rc.App.Aliases)
	}
	q := music.RewardsBasicQuery{Region: rc.Cmd.Region}
	mergeParams(rc.Cmd.Params, &q)
	q.Profile = rc.GetProfileCard()

	reason := ""

	if target := rc.GetSelfTarget(); target != nil && target.Binding != nil {
		if !hasUsableSuiteData(target.Binding) {
			reason = "当前账号没有可用的 Suite 抓包数据"
		} else if snapshot := rc.ResolveSnapshot(false); snapshot != nil {
			detailQuery := music.RewardsDetailQuery{
				Region:        q.Region,
				Title:         q.Title,
				TitleStyle:    q.TitleStyle,
				JewelIconPath: q.JewelIconPath,
				ShardIconPath: q.ShardIconPath,
				Profile:       q.Profile,
			}
			if _, buildErr := musicCtrl.BuildMusicRewardsDetailRequestFromSnapshot(detailQuery, snapshot); buildErr == nil {
				return musicCtrl.RenderMusicRewardsDetailFromSnapshot(detailQuery, snapshot)
			} else if strings.Contains(buildErr.Error(), "unavailable") {
				reason = "无法获取 Suite 快照中的成绩数据"
			} else {
				reason = "Suite 快照中的成绩数据无法解析"
			}
		} else {
			reason = "无法解析当前账号的 Suite 快照"
		}
	}

	var clearCounts []sekaiutils.AnotherUserMusicDifficultyClearCount
	if resp := rc.GetPublicProfileResponse(); resp != nil {
		clearCounts = resp.UserMusicDifficultyClearCount
	}

	return musicCtrl.RenderMusicRewardsBasicEstimate(q, clearCounts, reason)
}
