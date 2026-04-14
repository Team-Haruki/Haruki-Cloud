package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	sekaidb "haruki-cloud/database/sekai"
	gamecharacterdb "haruki-cloud/database/sekai/gamecharacter"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/card"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/query"
	sekaiutils "haruki-cloud/utils/sekai"
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

	resp, err := sekaiutils.GetSekaiAPIClient().GetUserProfile(region, pjskUserID)
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

func defaultEnabledDiffs() []sekaiutils.MusicDifficultyType {
	return []sekaiutils.MusicDifficultyType{
		sekaiutils.MusicDifficultyMaster,
		sekaiutils.MusicDifficultyExpert,
	}
}

func formatArrestText(resp *sekaiutils.GetAnotherProfileResponse, diffs []sekaiutils.MusicDifficultyType, challengeCharacterName string, uidVisible bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("逮捕: %s (UID: %s) Lv.%d\n",
		resp.User.Name, arrestDisplayUID(resp.User.UserID, uidVisible), resp.User.Rank))

	countByDiff := make(map[sekaiutils.MusicDifficultyType]sekaiutils.AnotherUserMusicDifficultyClearCount)
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

func resolveEducationAreaCharacterID(ctx context.Context, app *renderapp.App, region renderregion.Value, query string) (int, error) {
	return resolveGameCharacterIDByQuery(ctx, app, region, query, "education area")
}

func resolveEducationBondsCharacterID(ctx context.Context, app *renderapp.App, region renderregion.Value, query string) (int, error) {
	return resolveGameCharacterIDByQuery(ctx, app, region, query, "education bonds")
}

func resolveGameCharacterIDByQuery(
	ctx context.Context,
	app *renderapp.App,
	region renderregion.Value,
	query string,
	serviceLabel string,
) (int, error) {
	if app == nil || app.Sekai == nil {
		return 0, fmt.Errorf("%s service unavailable: sekai client not configured", serviceLabel)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, fmt.Errorf("请输入角色名")
	}

	target := normalizeGameCharacterText(query)
	if target == "" {
		return 0, fmt.Errorf("请输入角色名")
	}
	if app.Aliases != nil {
		if charID, ok, err := app.Aliases.TryResolveCharacterID(ctx, query); err != nil {
			return 0, err
		} else if ok && charID > 0 {
			return charID, nil
		}
	}
	if charID, ok := card.ResolveDefaultCharacterNickname(query); ok && charID > 0 {
		return charID, nil
	}

	rows, err := app.Sekai.Gamecharacter.Query().
		Where(gamecharacterdb.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query %s characters failed: %w", serviceLabel, err)
	}
	ids := matchGameCharacterIDs(rows, target)
	if len(ids) == 0 {
		rows, err = app.Sekai.Gamecharacter.Query().
			Where(gamecharacterdb.GameIDGT(0)).
			All(ctx)
		if err != nil {
			return 0, fmt.Errorf("query %s characters failed: %w", serviceLabel, err)
		}
		ids = matchGameCharacterIDs(rows, target)
	}

	switch len(ids) {
	case 0:
		return 0, fmt.Errorf("未找到角色：%s", query)
	case 1:
		return ids[0], nil
	default:
		return 0, fmt.Errorf("匹配到多个角色：%s", query)
	}
}

func matchGameCharacterIDs(rows []*sekaidb.Gamecharacter, target string) []int {
	if target == "" {
		return nil
	}

	matched := make(map[int]struct{})
	for _, row := range rows {
		if row == nil || row.GameID <= 0 {
			continue
		}
		for _, candidate := range gameCharacterNames(row) {
			if normalizeGameCharacterText(candidate) != target {
				continue
			}
			matched[int(row.GameID)] = struct{}{}
			break
		}
	}
	if len(matched) == 0 {
		return nil
	}

	ids := make([]int, 0, len(matched))
	for id := range matched {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func gameCharacterNames(row *sekaidb.Gamecharacter) []string {
	values := make([]string, 0, 8)
	appendGameCharacterName(&values, strings.TrimSpace(row.FirstName+row.GivenName))
	appendGameCharacterName(&values, strings.TrimSpace(strings.TrimSpace(row.FirstName)+" "+strings.TrimSpace(row.GivenName)))
	appendGameCharacterName(&values, strings.TrimSpace(row.FirstNameEnglish+row.GivenNameEnglish))
	appendGameCharacterName(&values, strings.TrimSpace(strings.TrimSpace(row.FirstNameEnglish)+" "+strings.TrimSpace(row.GivenNameEnglish)))
	appendGameCharacterName(&values, row.FirstName)
	appendGameCharacterName(&values, row.GivenName)
	appendGameCharacterName(&values, row.FirstNameEnglish)
	appendGameCharacterName(&values, row.GivenNameEnglish)
	return values
}

func appendGameCharacterName(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *values {
		if strings.EqualFold(existing, value) {
			return
		}
	}
	*values = append(*values, value)
}

func normalizeGameCharacterText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), ""))
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
