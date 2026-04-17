package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	sekaidb "haruki-cloud/database/sekai"
	gamecharacterdb "haruki-cloud/database/sekai/gamecharacter"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/card"
)

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
