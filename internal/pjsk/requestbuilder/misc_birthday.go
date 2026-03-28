package requestbuilder

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	sekaidb "haruki-cloud/database/sekai"
	sekaicard "haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/gamecharacterunit"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

type miscBirthdaySelection struct {
	Cid           int    `json:"cid,omitempty"`
	UpcomingIndex int    `json:"upcoming_index,omitempty"`
	Query         string `json:"query,omitempty"`
}

type birthdayDate struct {
	Month int
	Day   int
}

type birthdayCharacterInfo struct {
	Cid   int
	Month int
	Day   int
	Next  time.Time
}

var (
	birthdayDisplayLocation = time.FixedZone("UTC+8", 8*3600)
	birthdayRegionOffsets   = map[renderregion.Value]int{
		renderregion.JP: 9,
		renderregion.CN: 8,
		renderregion.TW: 8,
		renderregion.EN: 0,
		renderregion.KR: 9,
	}
	birthdayRegionNames = map[renderregion.Value]string{
		renderregion.JP: "日服",
		renderregion.CN: "国服",
		renderregion.TW: "台服",
		renderregion.EN: "国际服",
		renderregion.KR: "韩服",
	}
	birthdayFifthAnnivRegions = map[renderregion.Value]struct{}{
		renderregion.JP: {},
	}
	characterBirthdays = map[int]birthdayDate{
		1:  {Month: 8, Day: 11},
		2:  {Month: 5, Day: 9},
		3:  {Month: 10, Day: 27},
		4:  {Month: 1, Day: 8},
		5:  {Month: 4, Day: 14},
		6:  {Month: 10, Day: 5},
		7:  {Month: 3, Day: 19},
		8:  {Month: 12, Day: 6},
		9:  {Month: 3, Day: 2},
		10: {Month: 7, Day: 26},
		11: {Month: 11, Day: 12},
		12: {Month: 5, Day: 25},
		13: {Month: 5, Day: 17},
		14: {Month: 9, Day: 9},
		15: {Month: 7, Day: 20},
		16: {Month: 6, Day: 24},
		17: {Month: 2, Day: 10},
		18: {Month: 1, Day: 27},
		19: {Month: 4, Day: 30},
		20: {Month: 8, Day: 27},
		21: {Month: 8, Day: 31},
		22: {Month: 12, Day: 27},
		23: {Month: 12, Day: 27},
		24: {Month: 1, Day: 30},
		25: {Month: 11, Day: 5},
		26: {Month: 2, Day: 17},
	}
	miscBirthdayDefaultNicknames = map[string]int{
		"ick": 1, "ichika": 1, "星乃一歌": 1,
		"saki": 2, "咲希": 2, "天马咲希": 2,
		"hnm": 3, "honami": 3, "穗波": 3,
		"shiho": 4, "志步": 4, "日野森志步": 4,
		"mnr": 5, "minori": 5, "实乃理": 5, "花里みのり": 5,
		"hrk": 6, "haruka": 6, "遥": 6,
		"airi": 7, "爱莉": 7, "桃井爱莉": 7,
		"szk": 8, "shizuku": 8, "雫": 8,
		"kohane": 9, "小豆泽心羽": 9,
		"an": 10, "杏": 10, "白石杏": 10,
		"akito": 11, "彰人": 11, "青柳彰人": 11,
		"toya": 12, "冬弥": 12, "天马冬弥": 12,
		"tsks": 13, "tsukasa": 13, "司": 13,
		"emu": 14, "笑梦": 14, "天马笑梦": 14,
		"nene": 15, "宁宁": 15, "楠宁宁": 15,
		"rui": 16, "类": 16, "神代类": 16,
		"knd": 17, "kanade": 17, "奏": 17,
		"mfy": 18, "mafuyu": 18, "真冬": 18,
		"ena": 19, "绘名": 19, "朝比奈绘名": 19,
		"mzk": 20, "mizuki": 20, "瑞希": 20, "晓山瑞希": 20,
		"miku": 21, "初音": 21, "初音未来": 21,
		"rin": 22, "镜音铃": 22,
		"len": 23, "镜音连": 23,
		"luka": 24, "巡音流歌": 24,
		"meiko": 25,
		"kaito": 26,
	}
)

func BuildMiscBirthdayRequest(r *parser.ResolvedCommand, app *renderapp.App) (*drawing.CharaBirthdayRequest, error) {
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("misc birthday service unavailable: sekai client not configured")
	}

	region := renderregion.WithDefault(renderregion.Normalize(r.Region))
	selection, err := normalizeBirthdaySelection(r)
	if err != nil {
		return nil, err
	}
	if selection.Cid == 0 && strings.TrimSpace(selection.Query) != "" {
		selection.Cid, err = resolveBirthdayCharacterID(app, region, selection.Query)
		if err != nil {
			return nil, err
		}
	}

	infos := buildBirthdayInfos(region, time.Now())
	info, err := selectBirthdayInfo(infos, selection)
	if err != nil {
		return nil, err
	}

	cards, cardImagePath, err := loadBirthdayCards(app, region, info.Cid)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(birthdayDisplayLocation)
	nextTime := info.Next.In(birthdayDisplayLocation)
	isFifthAnniv := isBirthdayFifthAnniv(region)

	gachaStart, gachaEnd := nextTime, nextTime.AddDate(0, 0, 7)
	liveStart, liveEnd := nextTime, nextTime.AddDate(0, 0, 1)

	req := &drawing.CharaBirthdayRequest{
		Cid:               info.Cid,
		Month:             info.Month,
		Day:               info.Day,
		RegionName:        birthdayRegionName(region),
		DaysUntilBirthday: birthdayDaysUntil(now, nextTime),
		ColorCode:         resolveBirthdayColorCode(app, region, info.Cid),
		SdImagePath: assets.ResolveRegionAssetPath(
			app.Assets,
			region.String(),
			fmt.Sprintf("character/character_sd_l/chr_sp_%d.png", info.Cid),
		),
		TitleImagePath: assets.ResolveRegionAssetPath(
			app.Assets,
			region.String(),
			fmt.Sprintf("character/label_horizontal/chr_h_lb_%d.png", info.Cid),
		),
		CardImagePath: cardImagePath,
		Cards:         cards,
		IsFifthAnniv:  isFifthAnniv,
		GachaTime:     buildBirthdayEventTime(now, gachaStart, gachaEnd),
		LiveTime:      buildBirthdayEventTime(now, liveStart, liveEnd),
		AllCharacters: buildBirthdayCalendar(app, infos),
	}

	if isFifthAnniv {
		gachaStart = nextTime.AddDate(0, 0, -4)
		gachaEnd = nextTime.AddDate(0, 0, 3)
		req.GachaTime = buildBirthdayEventTime(now, gachaStart, gachaEnd)

		dropTime := buildBirthdayEventTime(now, nextTime.AddDate(0, 0, -3), nextTime)
		flowerTime := buildBirthdayEventTime(now, nextTime.AddDate(0, 0, -3), nextTime.AddDate(0, 0, 3))
		partyTime := buildBirthdayEventTime(now, nextTime, nextTime.AddDate(0, 0, 3))

		req.DropTime = &dropTime
		req.FlowerTime = &flowerTime
		req.PartyTime = &partyTime
	}

	return req, nil
}

func normalizeBirthdaySelection(r *parser.ResolvedCommand) (miscBirthdaySelection, error) {
	selection := miscBirthdaySelection{}
	if r != nil {
		mergeParams(r.Params, &selection)
	}
	selection.Query = strings.TrimSpace(selection.Query)
	if selection.Cid > 0 || selection.UpcomingIndex > 0 || selection.Query != "" {
		return selection, nil
	}

	rawQuery := ""
	if r != nil {
		rawQuery = strings.TrimSpace(r.Query)
	}
	if rawQuery == "" {
		selection.UpcomingIndex = 1
		return selection, nil
	}

	if index, err := strconv.Atoi(rawQuery); err == nil {
		if index <= 0 {
			return miscBirthdaySelection{}, fmt.Errorf("角色生日索引超出范围")
		}
		selection.UpcomingIndex = index
		return selection, nil
	}

	selection.Query = rawQuery
	return selection, nil
}

func buildBirthdayInfos(region renderregion.Value, now time.Time) []birthdayCharacterInfo {
	infos := make([]birthdayCharacterInfo, 0, len(characterBirthdays))
	for cid, birthday := range characterBirthdays {
		infos = append(infos, birthdayCharacterInfo{
			Cid:   cid,
			Month: birthday.Month,
			Day:   birthday.Day,
			Next:  nextBirthdayTime(region, birthday.Month, birthday.Day, now),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Next.Equal(infos[j].Next) {
			return infos[i].Cid < infos[j].Cid
		}
		return infos[i].Next.Before(infos[j].Next)
	})
	return infos
}

func selectBirthdayInfo(infos []birthdayCharacterInfo, selection miscBirthdaySelection) (birthdayCharacterInfo, error) {
	if selection.Cid > 0 {
		for _, info := range infos {
			if info.Cid == selection.Cid {
				return info, nil
			}
		}
		return birthdayCharacterInfo{}, fmt.Errorf("invalid birthday request")
	}

	index := selection.UpcomingIndex
	if index <= 0 {
		index = 1
	}
	if index > len(infos) {
		return birthdayCharacterInfo{}, fmt.Errorf("角色生日索引超出范围")
	}
	return infos[index-1], nil
}

func resolveBirthdayCharacterID(app *renderapp.App, region renderregion.Value, query string) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, fmt.Errorf("请输入角色名")
	}

	extractor := parser.NewExtractor(miscBirthdayDefaultNicknames)
	if result := extractor.ExtractCharacter(query); result.Found && strings.TrimSpace(result.Remaining) == "" {
		return result.Value, nil
	}

	ids, err := lookupBirthdayCharacterIDs(app, region, query)
	if err != nil {
		return 0, err
	}
	switch len(ids) {
	case 0:
		return 0, fmt.Errorf("未找到对应角色: %s", query)
	case 1:
		return ids[0], nil
	default:
		return 0, fmt.Errorf("角色名存在歧义: %s", query)
	}
}

func lookupBirthdayCharacterIDs(app *renderapp.App, region renderregion.Value, query string) ([]int, error) {
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("misc birthday service unavailable: sekai client not configured")
	}

	rows, err := app.Sekai.Gamecharacter.Query().
		Where(gamecharacter.ServerRegionEQ(region.String())).
		All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query birthday characters failed: %w", err)
	}
	ids := matchBirthdayCharacterIDs(rows, query)
	if len(ids) > 0 {
		return ids, nil
	}

	rows, err = app.Sekai.Gamecharacter.Query().
		Where(gamecharacter.GameIDGT(0)).
		All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query birthday characters failed: %w", err)
	}
	return matchBirthdayCharacterIDs(rows, query), nil
}

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

func loadBirthdayCards(app *renderapp.App, region renderregion.Value, cid int) ([]drawing.CharaBirthdayCard, string, error) {
	entities, err := app.Sekai.Card.Query().
		Where(
			sekaicard.ServerRegionEQ(region.String()),
			sekaicard.CharacterIDEQ(int64(cid)),
			sekaicard.CardRarityTypeEQ("rarity_birthday"),
		).
		Order(sekaicard.ByReleaseAt()).
		All(context.Background())
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
			filepath.Join("character", "member", assetBundleName+"_rip", "card_normal.png"),
		),
	)
}

func resolveBirthdayColorCode(app *renderapp.App, region renderregion.Value, cid int) string {
	entity, err := app.Sekai.Gamecharacterunit.Query().
		Where(
			gamecharacterunit.ServerRegionEQ(region.String()),
			gamecharacterunit.GameCharacterIDEQ(int64(cid)),
		).
		Order(gamecharacterunit.ByID()).
		First(context.Background())
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
