package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	gamecharacterdb "haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/common"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/usererror"
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
		return p, usererror.BadParam(uidArg, i18n.M("profile.target_invalid"))
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
	return p, usererror.Forbidden(i18n.M("common.self_only"))
}

func (sekaiHandlers) ArrestHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		ParseUIDArg: common.BoolPtr(true),
		Commands: []string{
			"/逮捕", "/pjsk逮捕", "/pjsk arrest",
		},
		Path: "arrest",
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
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
		Commands: []string{
			"/注册时间", "/pjsk reg time", "/pjsk 注册时间", "/查时间",
		},
		Path: "profile/reg-time",
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

	resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, pjskUserID)
	if err != nil {
		return nil, playerProfileFetchError(err)
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
	lines := []i18n.Message{i18n.M("misc.arrest.header", i18n.Data{
		"Name": resp.User.Name,
		"UID":  arrestDisplayUID(resp.User.UserID, uidVisible),
		"Rank": resp.User.Rank,
	})}

	countByDiff := make(map[sekaiapi.MusicDifficultyType]sekaiapi.AnotherUserMusicDifficultyClearCount)
	for _, c := range resp.UserMusicDifficultyClearCount {
		countByDiff[c.MusicDifficultyType] = c
	}

	for _, diff := range diffs {
		c, ok := countByDiff[diff]
		if !ok {
			continue
		}
		lines = append(lines, i18n.M("misc.arrest.difficulty", i18n.Data{
			"Difficulty": i18n.DifficultyLabel(string(diff)),
			"Clear":      c.LiveClear,
			"FC":         c.FullCombo,
			"AP":         c.AllPerfect,
		}))
	}

	if resp.UserChallengeLiveSoloResult.HighScore > 0 {
		lines = append(lines, i18n.M("misc.arrest.challenge", i18n.Data{
			"Character": arrestChallengeCharacterLabel(resp.UserChallengeLiveSoloResult.CharacterID, challengeCharacterName),
			"Score":     i18n.Thousands(int64(resp.UserChallengeLiveSoloResult.HighScore)),
		}))
	}

	return i18n.LinesText(lines)
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

func arrestChallengeCharacterLabel(characterID int, resolvedName string) i18n.Message {
	if name := strings.TrimSpace(resolvedName); name != "" {
		return i18n.Verbatim(name)
	}
	return i18n.M("misc.arrest.character_id", i18n.Data{"ID": characterID})
}

func arrestDisplayUID(uid int64, visible bool) string {
	return i18n.MaskUID(strconv.FormatInt(uid, 10), visible)
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
	loc, _ := displaytime.LoadLocation(timeZone)
	regTime := time.Unix(ts, 0)
	text := i18n.T("misc.reg_time.result", i18n.Data{
		"UID":  i18n.MaskUID(pjskUserID, target.Visible),
		"Time": i18n.FormatUserTime(regTime, loc),
		"Ago":  displaytime.FormatRelativeDuration(time.Since(regTime)),
	})
	return onebot11.Message{onebot11.Text(text)}, nil
}

func calcRegistrationTime(userID string, server string) (int64, error) {
	switch renderregion.Normalize(server) {
	case renderregion.JP, renderregion.EN:
		if len(userID) <= 3 {
			return 0, usererror.BadParam(userID, i18n.M("common.param.uid_digits"))
		}
		n, err := strconv.ParseInt(userID[:len(userID)-3], 10, 64)
		if err != nil {
			return 0, usererror.BadParam(userID, i18n.M("common.param.uid_digits")).WithCause(err)
		}
		return 1600218000 + int64(float64(n)/(1024*4096)), nil
	case renderregion.TW, renderregion.KR, renderregion.CN:
		n, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return 0, usererror.BadParam(userID, i18n.M("common.param.uid_digits")).WithCause(err)
		}
		return int64(float64(n) / (1024 * 1024 * 4096)), nil
	default:
		return 0, usererror.Invalid(i18n.M("profile.registration.unsupported_region"))
	}
}
