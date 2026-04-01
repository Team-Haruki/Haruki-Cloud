package handler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/drawing"
	sekaiutils "haruki-cloud/utils/sekai"
)

func executeMusic(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App == nil || rc.App.Music == nil {
		return nil, fmt.Errorf("music service unavailable: music controller is not configured")
	}
	if rc.App.Aliases != nil {
		rc.App.Music.SetAliasResolver(rc.App.Aliases)
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "music-detail":
		q := music.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = rc.App.Music.RenderMusicDetail(q)
	case "music-list":
		q := music.ListQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if strings.TrimSpace(q.Keyword) == "" {
			q.Keyword = strings.TrimSpace(rc.Cmd.Query)
		}
		q.DetailedProfile = rc.GetDetailedProfile()
		data, err = rc.App.Music.RenderMusicList(q)
	case "music-chart":
		q := music.ChartQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = rc.App.Music.RenderMusicChart(q)
	case "music-progress":
		q := music.ProgressQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = rc.GetProfileCard()
		data, err = rc.App.Music.RenderMusicProgress(q)
	case "music-rewards":
		data, err = renderMusicRewards(rc.Ctx, rc.Cmd, rc.App, rc.GetProfileCard())
	case "music-note-count":
		q := music.NoteCountQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		matches, resolveErr := rc.App.Music.FindMusicChartsByNoteCount(q)
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
		result, resolveErr := rc.App.Music.ResolveMusicCover(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(result.JacketPath, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		text := fmt.Sprintf("【%d】%s", result.Music.ID, result.Music.Title)
		return append(image, onebot11.Text(text)), nil
	case "music-bpm":
		q := music.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := rc.App.Music.ResolveMusicBPM(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(result.JacketPath, rc.App, BotModulePJSK)
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
		return nil, fmt.Errorf("bridge: unsupported music mode %q", rc.Cmd.Mode)
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

func renderMusicRewards(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App, publicProfileCard *drawing.ProfileCardRequest) ([]byte, error) {
	q := music.RewardsBasicQuery{Region: r.Region}
	mergeParams(r.Params, &q)
	q.Profile = publicProfileCard

	reason := ""
	region := regionWithDefault(r.Region)

	if strings.TrimSpace(r.RequesterPlatform) != "" && strings.TrimSpace(r.RequesterUserID) != "" {
		queryParams := userQueryParams{
			Mode:           "self",
			Platform:       strings.TrimSpace(r.RequesterPlatform),
			PlatformUserID: strings.TrimSpace(r.RequesterUserID),
		}
		target, err := resolveGameTarget(ctx, queryParams, region, r.RegionExplicit, app)
		if err == nil && target.Binding != nil {
			if !hasUsableSuiteData(target.Binding) {
				reason = "当前账号没有可用的 Suite 抓包数据"
			} else if uid, convErr := strconv.ParseInt(target.PJSKUserID, 10, 64); convErr == nil {
				raw, toolboxErr := sekaiutils.GetToolboxClient().GetPrivateDataValue(
					region, sekaiutils.ToolboxDataTypeSuite, uid, queryParams.Platform, queryParams.PlatformUserID, "userMusicAchievements")
				if toolboxErr == nil && len(raw) > 0 {
					detailQuery := music.RewardsDetailQuery{
						Region:        q.Region,
						Title:         q.Title,
						TitleStyle:    q.TitleStyle,
						JewelIconPath: q.JewelIconPath,
						ShardIconPath: q.ShardIconPath,
						Profile:       q.Profile,
					}
					if _, buildErr := app.Music.BuildMusicRewardsDetailRequestFromAchievements(detailQuery, raw); buildErr == nil {
						return app.Music.RenderMusicRewardsDetailFromAchievements(detailQuery, raw)
					}
					reason = "Suite 抓包中的成绩数据无法解析"
				} else {
					reason = "无法获取 Suite 抓包中的成绩数据"
				}
			}
		}
	}

	var clearCounts []sekaiutils.AnotherUserMusicDifficultyClearCount
	if publicProfileCard != nil && publicProfileCard.Profile != nil {
		if userID := strings.TrimSpace(publicProfileCard.Profile.ID); userID != "" {
			if resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, userID); err == nil && resp != nil {
				clearCounts = resp.UserMusicDifficultyClearCount
			}
		}
	}

	return app.Music.RenderMusicRewardsBasicEstimate(q, clearCounts, reason)
}
