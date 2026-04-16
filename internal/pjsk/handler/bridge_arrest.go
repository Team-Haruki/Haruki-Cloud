package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/onebot11"
	gamecharacterdb "haruki-cloud/database/sekai/gamecharacter"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/query"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

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

	// Censor user-controlled text (name shown in text output).
	if rc.App.Censor != nil {
		if !rc.App.Censor.CensorName(rc.Ctx, harukiUserID, pjskUserID, resp.User.Name, region) {
			resp.User.Name = ""
		}
	}
	queryClient := query.NewClient(nil, nil, rc.App.PJSK, nil)
	// Load the caller's enabled difficulties for self-mode; default for others.
	enabledDiffs := defaultEnabledDiffs()
	if p.Mode == "self" && harukiUserID > 0 && rc.App.PJSK != nil {
		if settings, sErr := queryClient.GetPJSKSettings(rc.Ctx, harukiUserID); sErr == nil && settings != nil {
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
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "jp":
		return 0
	case "cn":
		return 1
	case "tw":
		return 2
	case "en":
		return 3
	case "kr":
		return 4
	default:
		return 999
	}
}

// formatInt formats an integer with comma separators (e.g. 3011947 → "3,011,947").
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
