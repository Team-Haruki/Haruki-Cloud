package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gamecharacterdb "haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/common"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"
)

// UserQueryParams holds the resolved identity context for commands that query
// another user's data (arrest, registration time, etc.).
type UserQueryParams struct {
	Mode            string `json:"mode"`               // "self", "at_user", "uid"
	Platform        string `json:"platform"`           // caller's IM platform
	PlatformUserID  string `json:"platform_user_id"`   // caller's platform UID (self mode)
	AtUserID        string `json:"at_user_id"`         // @-mentioned platform UID (at_user mode)
	PJSKUserID      string `json:"pjsk_user_id"`       // direct game UID (uid mode)
	Selector        string `json:"selector,omitempty"` // u[i] binding selector (self mode only)
	ProfileVertical *bool  `json:"profile_vertical,omitempty"`
}

// isBindingSelector returns true if the value is a u[i] binding selector (e.g. "u1", "u2").
func isBindingSelector(value string) bool {
	if len(value) < 2 {
		return false
	}
	if value[0] != 'u' && value[0] != 'U' {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func resolveUserQueryParams(ctx HarrukiSekaiHandlerContext) (UserQueryParams, error) {
	p := UserQueryParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
	}
	uidArg := ctx.UIDArg()
	switch {
	case uidArg == "":
		p.Mode = "self"
	case isBindingSelector(uidArg):
		p.Mode = "self"
		p.Selector = uidArg
	case strings.HasPrefix(uidArg, "@"):
		p.Mode = "at_user"
		p.AtUserID = uidArg[1:] // strip "@"
	case isDigits(uidArg):
		p.Mode = "uid"
		p.PJSKUserID = uidArg
	default:
		return p, onebot11.NewReplayError("无效的参数：%q\n使用方式：%s [@用户 | 游戏ID | u序号]", uidArg, ctx.originalTriggerCmd)
	}
	return p, nil
}

// resolveSelfOnlyQueryParams is like resolveUserQueryParams but restricts to
// self-mode only (with optional u[i] selector). Used by commands that should
// not support @mention or direct UID queries (e.g. sud, msd).
func resolveSelfOnlyQueryParams(ctx HarrukiSekaiHandlerContext) (UserQueryParams, error) {
	p := UserQueryParams{
		Platform:       ctx.GetPlatform(),
		PlatformUserID: ctx.GetUserId(),
		Mode:           "self",
	}
	uidArg := ctx.UIDArg()
	if uidArg == "" {
		return p, nil
	}
	if isBindingSelector(uidArg) {
		p.Selector = uidArg
		return p, nil
	}
	return p, onebot11.NewReplayError("此命令仅支持查询自己的数据\n使用方式：%s [u序号]", ctx.originalTriggerCmd)
}

func (sekaiHandlers) ArrestHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		ParseUIDArg: common.BoolPtr(true),
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/逮捕", "/pjsk逮捕", "/pjsk arrest",
			},
			Path: "arrest",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			logger.Debugf("uidArg: %s, event: %+v", ctx.UIDArg(), ctx.GetEvent().Message)
			p, err := resolveUserQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleArrest, "arrest", p), nil
		},
	}, executeArrest)
}

func (sekaiHandlers) RegTimeHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Commands: []string{
				"/注册时间", "/pjsk reg time", "/pjsk 注册时间", "/查时间",
			},
			Path: "profile/reg-time",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			p, err := resolveSelfOnlyQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleRegTime, "reg-time", p), nil
		},
	}, executeRegTime)
}

func executeArrest(rc *RequestContext) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)

	region := regionWithDefault(rc.Cmd.Region)

	target, err := resolveGameTarget(rc.Ctx, p, region, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, err
	}
	region = resolvedTargetRegion(region, target)
	harukiUserID := target.HarukiUserID
	pjskUserID := target.PJSKUserID
	visible := target.Visible

	resp, err := rc.App.SekaiAPI.GetUserProfile(region, pjskUserID)
	if err != nil {
		return nil, fmt.Errorf("获取玩家信息失败：%w", err)
	}

	if rc.App.Censor != nil {
		if !rc.App.Censor.CensorName(rc.Ctx, harukiUserID, pjskUserID, resp.User.Name, region) {
			resp.User.Name = ""
		}
	}
	enabledDiffs := defaultEnabledDiffs()
	if p.Mode == "self" && harukiUserID > 0 && rc.App.PJSK != nil {
		if settings, sErr := accountdata.GetUserSettings(rc.Ctx, rc.App.PJSK, harukiUserID); sErr == nil && settings != nil {
			if len(settings.PJSKEnabledDifficulties) > 0 {
				enabledDiffs = settings.PJSKEnabledDifficulties
			}
		}
	}
	text := formatArrestText(resp, enabledDiffs, resolveArrestChallengeCharacterName(rc.Ctx, rc.App, resp.UserChallengeLiveSoloResult.CharacterID), visible)
	return onebot11.Message{onebot11.Text(text)}, nil
}

func defaultEnabledDiffs() []sekaiapi.MusicDifficultyType {
	return []sekaiapi.MusicDifficultyType{
		sekaiapi.MusicDifficultyMaster,
		sekaiapi.MusicDifficultyExpert,
	}
}

func formatArrestText(resp *sekaiapi.GetAnotherProfileResponse, diffs []sekaiapi.MusicDifficultyType, challengeCharacterName string, uidVisible bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("逮捕: %s (UID: %s) Lv.%d\n",
		resp.User.Name, arrestDisplayUID(resp.User.UserID, uidVisible), resp.User.Rank))

	countByDiff := make(map[sekaiapi.MusicDifficultyType]sekaiapi.AnotherUserMusicDifficultyClearCount)
	for _, c := range resp.UserMusicDifficultyClearCount {
		countByDiff[c.MusicDifficultyType] = c
	}

	for _, diff := range diffs {
		c, ok := countByDiff[diff]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s] Clear:%d FC:%d AP:%d\n",
			diff, c.LiveClear, c.FullCombo, c.AllPerfect))
	}

	if resp.UserChallengeLiveSoloResult.HighScore > 0 {
		label := arrestChallengeCharacterLabel(resp.UserChallengeLiveSoloResult.CharacterID, challengeCharacterName)
		sb.WriteString(fmt.Sprintf("挑战Live(%s): %s分",
			label, formatInt(resp.UserChallengeLiveSoloResult.HighScore)))
	}

	return strings.TrimRight(sb.String(), "\n")
}

func resolveArrestChallengeCharacterName(ctx context.Context, app *renderapp.App, characterID int) string {
	if characterID <= 0 || app == nil || app.Sekai == nil {
		return ""
	}

	rows, err := app.Sekai.Gamecharacter.Query().
		Where(gamecharacterdb.GameIDEQ(int64(characterID))).
		All(ctx)
	if err != nil || len(rows) == 0 {
		return ""
	}

	bestName := ""
	bestRank := 999
	for _, row := range rows {
		candidates := []string{
			strings.TrimSpace(row.FirstName + row.GivenName),
			strings.TrimSpace(strings.TrimSpace(row.FirstName) + " " + strings.TrimSpace(row.GivenName)),
			strings.TrimSpace(row.FirstNameEnglish + row.GivenNameEnglish),
			strings.TrimSpace(strings.TrimSpace(row.FirstNameEnglish) + " " + strings.TrimSpace(row.GivenNameEnglish)),
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			rank := arrestCharacterRegionRank(row.ServerRegion)
			if rank < bestRank {
				bestRank = rank
				bestName = candidate
				break
			}
		}
	}
	return strings.TrimSpace(bestName)
}

func arrestChallengeCharacterLabel(characterID int, resolvedName string) string {
	if name := strings.TrimSpace(resolvedName); name != "" {
		return name
	}
	return fmt.Sprintf("角色ID:%d", characterID)
}

func arrestDisplayUID(uid int64, visible bool) string {
	return maskPJSKUID(strconv.FormatInt(uid, 10), visible)
}

func arrestCharacterRegionRank(region string) int {
	switch renderregion.Normalize(region) {
	case renderregion.JP:
		return 0
	case renderregion.CN:
		return 1
	case renderregion.TW:
		return 2
	case renderregion.EN:
		return 3
	case renderregion.KR:
		return 4
	default:
		return 999
	}
}

func formatInt(n int) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var buf strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		buf.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(s[i : i+3])
	}
	return buf.String()
}

func executeRegTime(rc *RequestContext) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)

	region := regionWithDefault(rc.Cmd.Region)

	target, err := resolveGameTarget(rc.Ctx, p, region, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, err
	}
	pjskUserID := target.PJSKUserID
	bindingServer := resolvedTargetRegion(region, target)

	ts, err := calcRegistrationTime(pjskUserID, bindingServer)
	if err != nil {
		return nil, err
	}

	timeZone := resolveHarukiUserTimeZone(rc.Ctx, rc.App, target.HarukiUserID)
	regTime := displaytime.TimeFromUnixSeconds(ts, timeZone)
	relDur := displaytime.FormatRelativeDuration(displaytime.Now(timeZone).Sub(displaytime.TimeFromUnixSeconds(ts, timeZone)))
	maskedUID := maskPJSKUID(pjskUserID, target.Visible)

	text := fmt.Sprintf("UID %s 注册时间如下\n%s (%s) (%s)",
		maskedUID, displaytime.FormatTime(regTime, "2006-01-02 15:04:05"), timeZone, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}

func calcRegistrationTime(userID string, server string) (int64, error) {
	switch renderregion.Normalize(server) {
	case renderregion.JP, renderregion.EN:
		if len(userID) <= 3 {
			return 0, fmt.Errorf("账号ID格式不正确")
		}
		n, err := strconv.ParseInt(userID[:len(userID)-3], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return 1600218000 + int64(float64(n)/(1024*4096)), nil
	case renderregion.TW, renderregion.KR, renderregion.CN:
		n, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return int64(float64(n) / (1024 * 1024 * 4096)), nil
	default:
		return 0, fmt.Errorf("不支持的服务器：%s", server)
	}
}
