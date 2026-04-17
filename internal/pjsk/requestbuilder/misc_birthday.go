package requestbuilder

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
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

func BuildMiscBirthdayRequest(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) (*drawing.CharaBirthdayRequest, error) {
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("misc birthday service unavailable: sekai client not configured")
	}
	if ctx == nil {
		ctx = context.TODO()
	}

	region := renderregion.WithDefault(renderregion.Normalize(r.Region))
	selection, err := normalizeBirthdaySelection(r)
	if err != nil {
		return nil, err
	}
	if selection.Cid == 0 && strings.TrimSpace(selection.Query) != "" {
		selection.Cid, err = resolveBirthdayCharacterID(ctx, app, region, selection.Query)
		if err != nil {
			return nil, err
		}
	}

	infos := buildBirthdayInfos(region, time.Now())
	info, err := selectBirthdayInfo(infos, selection)
	if err != nil {
		return nil, err
	}

	cards, cardImagePath, err := loadBirthdayCards(ctx, app, region, info.Cid)
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
		ColorCode:         resolveBirthdayColorCode(ctx, app, region, info.Cid),
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

		req.DropTime = new(buildBirthdayEventTime(now, nextTime.AddDate(0, 0, -3), nextTime))
		req.FlowerTime = new(buildBirthdayEventTime(now, nextTime.AddDate(0, 0, -3), nextTime.AddDate(0, 0, 3)))
		req.PartyTime = new(buildBirthdayEventTime(now, nextTime, nextTime.AddDate(0, 0, 3)))
	}

	return req, nil
}

func normalizeBirthdaySelection(r *parser.ResolvedCommand) (miscBirthdaySelection, error) {
	selection := miscBirthdaySelection{}
	if r != nil {
		MergeParams(r.Params, &selection)
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

func resolveBirthdayCharacterID(ctx context.Context, app *renderapp.App, region renderregion.Value, query string) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, fmt.Errorf("请输入角色名")
	}

	extractor := parser.NewExtractor(miscBirthdayDefaultNicknames)
	if result := extractor.ExtractCharacter(query); result.Found && strings.TrimSpace(result.Remaining) == "" {
		return result.Value, nil
	}

	ids, err := lookupBirthdayCharacterIDs(ctx, app, region, query)
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

func lookupBirthdayCharacterIDs(ctx context.Context, app *renderapp.App, region renderregion.Value, query string) ([]int, error) {
	if app == nil || app.Sekai == nil {
		return nil, fmt.Errorf("misc birthday service unavailable: sekai client not configured")
	}
	if ctx == nil {
		ctx = context.TODO()
	}

	rows, err := app.Sekai.Gamecharacter.Query().
		Where(gamecharacter.ServerRegionEQ(region.String())).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query birthday characters failed: %w", err)
	}
	ids := matchBirthdayCharacterIDs(rows, query)
	if len(ids) > 0 {
		return ids, nil
	}

	rows, err = app.Sekai.Gamecharacter.Query().
		Where(gamecharacter.GameIDGT(0)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query birthday characters failed: %w", err)
	}
	return matchBirthdayCharacterIDs(rows, query), nil
}
