package handler

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/music"
	renderuserdata "haruki-cloud/internal/pjsk/render/userdata"
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
		q := music.ListQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if strings.TrimSpace(q.Keyword) == "" {
			q.Keyword = strings.TrimSpace(rc.Cmd.Query)
		}
		q.DetailedProfile = rc.GetDetailedProfile()
		data, err = musicCtrl.RenderMusicList(q)
	case "music-chart":
		q := music.ChartQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = musicCtrl.RenderMusicChart(q)
	case "music-progress":
		q := music.ProgressQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		snapshot, snapshotErr := resolveRequiredSuiteSnapshot(rc)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		data, err = musicCtrl.RenderMusicProgressFromSnapshot(q, snapshot, rc.GetProfileCard())
	case "music-rewards":
		data, err = renderMusicRewards(rc)
	case "music-note-count":
		q := music.NoteCountQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		matches, resolveErr := musicCtrl.FindMusicChartsByNoteCount(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		lines := make([]string, 0, len(matches))
		for _, item := range matches {
			lines = append(lines, fmt.Sprintf("【%d】%s - %s %d", item.Music.ID, item.Music.Title, strings.ToUpper(item.Difficulty), item.PlayLevel))
		}
		return onebot11.Message{onebot11.Text(strings.Join(lines, "\n"))}, nil
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
		q := music.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := musicCtrl.ResolveMusicBPM(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(rc.Ctx, result.JacketPath, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		textLines := []string{
			fmt.Sprintf("【%d】%s", result.Music.ID, result.Music.Title),
			fmt.Sprintf("主 BPM: %s", formatMusicBPM(result.MainBPM)),
			fmt.Sprintf("BPM 变化: %s", formatBPMEvents(result.Events)),
			fmt.Sprintf("谱面来源: %s", strings.ToUpper(result.Difficulty)),
		}
		return append(image, onebot11.Text(strings.Join(textLines, "\n"))), nil
	default:
		return nil, unsupportedModeError("music", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func formatBPMEvents(events []music.BPMEvent) string {
	if len(events) == 0 {
		return "无数据"
	}
	parts := make([]string, 0, len(events))
	for _, item := range events {
		parts = append(parts, formatMusicBPM(item.BPM))
	}
	return strings.Join(parts, " -> ")
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

func resolveRequiredSuiteSnapshot(rc *RequestContext) (renderuserdata.Snapshot, error) {
	if rc == nil {
		return nil, fmt.Errorf(ErrMsgSuiteDataUnavailable)
	}

	target := rc.GetSelfTarget()
	if target == nil || !hasUsableSuiteData(target.Binding) {
		return nil, fmt.Errorf(ErrMsgSuiteDataUnavailable)
	}

	snapshot := rc.ResolveSnapshot(false)
	if snapshot == nil {
		return nil, fmt.Errorf("无法解析当前账号的 Suite 快照")
	}
	return snapshot, nil
}
