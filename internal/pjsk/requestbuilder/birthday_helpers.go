package requestbuilder

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	sekaicard "haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/gamecharacterunit"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func matchBirthdayCharacterIDs(rows []*sekaidb.Gamecharacter, query string) []int {
	target := normalizeBirthdayCharacterText(query)
	if target == "" {
		return nil
	}

	matched := make(map[int]struct{})
	for _, row := range rows {
		if row == nil || row.GameID <= 0 {
			continue
		}
		if !birthdayCharacterMatches(row, target) {
			continue
		}
		matched[int(row.GameID)] = struct{}{}
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

func birthdayCharacterMatches(row *sekaidb.Gamecharacter, target string) bool {
	for _, candidate := range birthdayCharacterNames(row) {
		if normalizeBirthdayCharacterText(candidate) == target {
			return true
		}
	}
	return false
}

func birthdayCharacterNames(row *sekaidb.Gamecharacter) []string {
	values := make([]string, 0, 8)
	appendBirthdayCharacterName(&values, strings.TrimSpace(row.FirstName+row.GivenName))
	appendBirthdayCharacterName(&values, strings.TrimSpace(strings.TrimSpace(row.FirstName)+" "+strings.TrimSpace(row.GivenName)))
	appendBirthdayCharacterName(&values, strings.TrimSpace(row.FirstNameEnglish+row.GivenNameEnglish))
	appendBirthdayCharacterName(&values, strings.TrimSpace(strings.TrimSpace(row.FirstNameEnglish)+" "+strings.TrimSpace(row.GivenNameEnglish)))
	appendBirthdayCharacterName(&values, row.FirstName)
	appendBirthdayCharacterName(&values, row.GivenName)
	appendBirthdayCharacterName(&values, row.FirstNameEnglish)
	appendBirthdayCharacterName(&values, row.GivenNameEnglish)
	return values
}

func appendBirthdayCharacterName(values *[]string, value string) {
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

func normalizeBirthdayCharacterText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), ""))
}

func loadBirthdayCards(ctx context.Context, app *renderapp.App, region renderregion.Value, cid int) ([]drawing.CharaBirthdayCard, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entities, err := app.Sekai.Card.Query().
		Where(
			sekaicard.ServerRegionEQ(region.String()),
			sekaicard.CharacterIDEQ(int64(cid)),
			sekaicard.CardRarityTypeEQ("rarity_birthday"),
		).
		Order(sekaicard.ByReleaseAt()).
		All(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("query birthday cards failed: %w", err)
	}
	if len(entities) == 0 {
		return nil, "", fmt.Errorf("birthday cards are required")
	}

	cards := make([]drawing.CharaBirthdayCard, 0, len(entities))
	cardImagePath := ""
	for _, entity := range entities {
		cardID := int(entity.GameID)
		if cardID <= 0 {
			cardID = entity.ID
		}

		thumbPath := birthdayRelativePath(app,
			assets.ResolveRegionAssetPath(
				app.Assets,
				region.String(),
				filepath.Join("thumbnail", "chara", entity.AssetbundleName+"_normal.png"),
			),
		)

		cards = append(cards, drawing.CharaBirthdayCard{
			ID:            cardID,
			ThumbnailPath: thumbPath,
		})

		cardImagePath = birthdayCardImagePath(app, region, entity.AssetbundleName)
		if cardImagePath == "" {
			cardImagePath = thumbPath
		}
	}

	return cards, cardImagePath, nil
}

func birthdayCardImagePath(app *renderapp.App, region renderregion.Value, assetBundleName string) string {
	if strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return birthdayRelativePath(app,
		assets.ResolveRegionAssetPath(
			app.Assets,
			region.String(),
			filepath.Join("character", "member", assetBundleName, "card_normal.png"),
		),
	)
}

func resolveBirthdayColorCode(ctx context.Context, app *renderapp.App, region renderregion.Value, cid int) string {
	if ctx == nil {
		ctx = context.Background()
	}
	entity, err := app.Sekai.Gamecharacterunit.Query().
		Where(
			gamecharacterunit.ServerRegionEQ(region.String()),
			gamecharacterunit.GameCharacterIDEQ(int64(cid)),
		).
		Order(gamecharacterunit.ByID()).
		First(ctx)
	if err == nil && strings.TrimSpace(entity.ColorCode) != "" {
		return entity.ColorCode
	}
	return "#FFFFFF"
}

func buildBirthdayCalendar(app *renderapp.App, infos []birthdayCharacterInfo) []drawing.CharaBirthdayData {
	items := make([]drawing.CharaBirthdayData, 0, len(infos))
	for _, info := range infos {
		items = append(items, drawing.CharaBirthdayData{
			Cid:      info.Cid,
			Month:    info.Month,
			Day:      info.Day,
			IconPath: birthdayRelativePath(app, charaIconPath(app.Assets, info.Cid)),
		})
	}
	return items
}

func nextBirthdayTime(region renderregion.Value, month, day int, now time.Time) time.Time {
	location := birthdayRegionLocation(region)
	regionNow := now.In(location)
	next := time.Date(regionNow.Year(), time.Month(month), day, 0, 0, 0, 0, location)
	if next.Before(regionNow) {
		next = time.Date(regionNow.Year()+1, time.Month(month), day, 0, 0, 0, 0, location)
	}
	return next.In(birthdayDisplayLocation)
}

func birthdayRegionLocation(region renderregion.Value) *time.Location {
	offset, ok := birthdayRegionOffsets[region]
	if !ok {
		offset = 9
	}
	return time.FixedZone(fmt.Sprintf("UTC%+d", offset), offset*3600)
}

func birthdayRegionName(region renderregion.Value) string {
	if name, ok := birthdayRegionNames[region]; ok {
		return name
	}
	if region.IsZero() {
		return "日服"
	}
	return strings.ToUpper(region.String())
}

func isBirthdayFifthAnniv(region renderregion.Value) bool {
	_, ok := birthdayFifthAnnivRegions[region]
	return ok
}

func birthdayDaysUntil(now, next time.Time) int {
	if !next.After(now) {
		return 0
	}
	return int(next.Sub(now).Hours() / 24)
}

func buildBirthdayEventTime(now, start, end time.Time) drawing.BirthdayEventTime {
	displayEnd := end.Add(-time.Minute)
	if displayEnd.Before(start) {
		displayEnd = end
	}
	return drawing.BirthdayEventTime{
		StartText: birthdayTimeText(now, start),
		EndText:   birthdayTimeText(now, displayEnd),
	}
}

func birthdayTimeText(now, target time.Time) string {
	target = target.In(birthdayDisplayLocation)
	return fmt.Sprintf("%s(%s)", target.Format("01-02 15:04"), birthdayReadableTime(now, target))
}

func birthdayReadableTime(now, target time.Time) string {
	diff := target.Sub(now)
	suffix := "后"
	if diff < 0 {
		suffix = "前"
		diff = -diff
	}

	seconds := int(diff.Seconds())
	switch {
	case seconds < 60:
		return fmt.Sprintf("%d秒%s", seconds, suffix)
	case seconds < 60*60:
		return fmt.Sprintf("%d分钟%s", seconds/60, suffix)
	case seconds < 60*60*24:
		return fmt.Sprintf("%d小时%d分钟%s", seconds/3600, seconds/60%60, suffix)
	default:
		return fmt.Sprintf("%d天%s", seconds/(60*60*24), suffix)
	}
}

func birthdayRelativePath(app *renderapp.App, target string) string {
	if strings.TrimSpace(target) == "" || app == nil || app.Assets == nil {
		return target
	}
	return assets.MakeRelative(app.Assets.Primary(), target)
}

func charaIconPath(helper *assets.AssetHelper, charID int) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
	}
	return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
}
