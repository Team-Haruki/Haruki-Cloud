package handler

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/music"
	sekaiutils "haruki-cloud/utils/sekai"
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
	groups := make([]musicLookupListGroup, 0)
	for _, item := range matches {
		if item.Music == nil {
			continue
		}
		groups = appendLookupListGroupItem(groups, item.Difficulty, music.ListItemQuery{
			MusicID:    item.Music.ID,
			Difficulty: item.Difficulty,
			Level:      item.PlayLevel,
		})
	}
	return renderMusicLookupListMessages(rc, musicCtrl, rc.Cmd.Region, "物量", strconv.Itoa(query.NoteCount), groups)
}

func renderBPMLookupListMessages(rc *RequestContext, musicCtrl *music.Controller, query music.BPMQuery, matches []music.BPMMatch) (onebot11.Message, error) {
	groups := make([]musicLookupListGroup, 0)
	for _, item := range matches {
		if item.Music == nil {
			continue
		}
		groups = appendLookupListGroupItem(groups, item.Difficulty, music.ListItemQuery{
			MusicID:    item.Music.ID,
			Difficulty: item.Difficulty,
		})
	}
	return renderMusicLookupListMessages(rc, musicCtrl, rc.Cmd.Region, "BPM", formatMusicBPM(query.BPM), groups)
}

type musicLookupListGroup struct {
	Difficulty string
	Items      []music.ListItemQuery
}

func renderMusicLookupListMessages(rc *RequestContext, musicCtrl *music.Controller, region string, prefix string, value string, groups []musicLookupListGroup) (onebot11.Message, error) {
	ordered := orderMusicLookupListGroups(groups)
	message := make(onebot11.Message, 0, len(ordered))
	for _, group := range ordered {
		if len(group.Items) == 0 {
			continue
		}
		title := buildMusicLookupListTitle(prefix, value, group.Difficulty)
		data, err := musicCtrl.RenderMusicList(music.ListQuery{
			Items:       group.Items,
			Difficulty:  group.Difficulty,
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
		message = append(message, image...)
	}
	if len(message) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}
	return message, nil
}

func appendLookupListGroupItem(groups []musicLookupListGroup, difficulty string, item music.ListItemQuery) []musicLookupListGroup {
	diff := strings.ToLower(strings.TrimSpace(difficulty))
	for i := range groups {
		if groups[i].Difficulty != diff {
			continue
		}
		groups[i].Items = append(groups[i].Items, item)
		return groups
	}
	return append(groups, musicLookupListGroup{
		Difficulty: diff,
		Items:      []music.ListItemQuery{item},
	})
}

func orderMusicLookupListGroups(groups []musicLookupListGroup) []musicLookupListGroup {
	order := []string{"easy", "normal", "hard", "expert", "master", "append"}
	result := make([]musicLookupListGroup, 0, len(groups))
	used := make(map[string]struct{}, len(groups))
	for _, diff := range order {
		for _, group := range groups {
			if group.Difficulty != diff {
				continue
			}
			result = append(result, group)
			used[diff] = struct{}{}
		}
	}
	for _, group := range groups {
		if _, ok := used[group.Difficulty]; ok {
			continue
		}
		result = append(result, group)
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
