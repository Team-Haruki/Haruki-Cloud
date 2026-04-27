package mysekai

import "strings"

type fixtureCategoryFilter struct {
	mainIDs map[int]struct{}
	subIDs  map[int]struct{}
}

type fixtureCategoryAliasRow struct {
	Name    string
	Aliases []string
}

var fixtureCategoryAliases = []fixtureCategoryAliasRow{
	{Name: "一般", Aliases: []string{"general", "normal", "普通", "通用", "家具"}},
	{Name: "小物", Aliases: []string{"small", "item", "小件", "摆件", "杂货", "装饰"}},
	{Name: "壁掛け", Aliases: []string{"wallhanging", "wallmounted", "壁挂", "壁饰", "挂件"}},
	{Name: "ディスプレイ", Aliases: []string{"display", "展示", "陈列"}},
	{Name: "キャンバス", Aliases: []string{"canvas", "画布", "画"}},
	{Name: "壁", Aliases: []string{"wall", "墙", "墙壁"}},
	{Name: "床", Aliases: []string{"floor", "地板", "地面", "地砖", "ゆか"}},
	{Name: "ラグ", Aliases: []string{"rug", "carpet", "地毯", "毯子", "毯"}},
	{Name: "家", Aliases: []string{"house", "home", "房子", "屋子"}},
	{Name: "道", Aliases: []string{"road", "path", "道路", "路", "小路"}},
	{Name: "柵", Aliases: []string{"fence", "栅栏", "围栏", "栏杆"}},
	{Name: "植物", Aliases: []string{"植物"}},
	{Name: "ぬいぐるみ", Aliases: []string{"plush", "doll", "stuffedtoy", "玩偶", "毛绒", "毛绒玩具", "娃娃"}},
	{Name: "カラータイル", Aliases: []string{"colortile", "color_tile", "彩色砖", "彩砖", "色块", "颜色砖"}},
	{Name: "ブロック", Aliases: []string{"block", "blocks", "积木", "方块", "块"}},
	{Name: "大型", Aliases: []string{"large", "largefixture", "大型", "大件"}},
	{Name: "ベッド", Aliases: []string{"bed", "床铺", "床具", "睡床"}},
	{Name: "テーブル", Aliases: []string{"table", "desk", "桌子", "桌", "茶几"}},
	{Name: "チェア", Aliases: []string{"chair", "seat", "椅子", "椅", "座椅"}},
	{Name: "ソファ", Aliases: []string{"sofa", "couch", "沙发"}},
	{Name: "棚", Aliases: []string{"shelf", "shelves", "rack", "架子", "货架", "柜", "柜子", "收纳", "収納物", "收纳物", "储物"}},
	{Name: "観葉植物", Aliases: []string{"pottedplant", "ornamentalplant", "盆栽", "盆景", "观叶植物", "绿植"}},
	{Name: "家電", Aliases: []string{"appliance", "appliances", "家电", "电器"}},
	{Name: "その他", Aliases: []string{"other", "others", "其他", "其它", "其他类"}},
	{Name: "木", Aliases: []string{"tree", "wood", "树", "树木", "木"}},
	{Name: "花", Aliases: []string{"花", "花朵", "flower", "flowers"}},
}

func resolveFixtureCategoryFilter(query string, mainGenreMap, subGenreMap map[int]map[string]any) (*fixtureCategoryFilter, error) {
	token := normalizeFixtureCategoryToken(query)
	if token == "" {
		return nil, nil
	}

	filter := &fixtureCategoryFilter{
		mainIDs: map[int]struct{}{},
		subIDs:  map[int]struct{}{},
	}

	for id, info := range mainGenreMap {
		if matchesFixtureCategoryToken(token, info) {
			filter.mainIDs[id] = struct{}{}
		}
	}
	for id, info := range subGenreMap {
		if matchesFixtureCategoryToken(token, info) {
			filter.subIDs[id] = struct{}{}
		}
	}
	if len(filter.mainIDs) == 0 && len(filter.subIDs) == 0 {
		return nil, &fixtureCategoryNotFoundError{query: strings.TrimSpace(query)}
	}
	return filter, nil
}

func (f *fixtureCategoryFilter) allows(mainGenreID, subGenreID int) bool {
	if f == nil {
		return true
	}
	if _, ok := f.mainIDs[mainGenreID]; ok {
		return true
	}
	if _, ok := f.subIDs[subGenreID]; ok {
		return true
	}
	return false
}

type fixtureCategoryNotFoundError struct {
	query string
}

func (e *fixtureCategoryNotFoundError) Error() string {
	if e == nil || strings.TrimSpace(e.query) == "" {
		return "mysekai fixture category not found"
	}
	return "mysekai fixture category not found: " + strings.TrimSpace(e.query)
}

func matchesFixtureCategoryToken(token string, info map[string]any) bool {
	for _, candidate := range fixtureCategoryTokens(info) {
		if token == candidate {
			return true
		}
	}
	return false
}

func fixtureCategoryTokens(info map[string]any) []string {
	name := stringValue(info["name"])
	assetbundleName := stringValue(info["assetbundleName"])

	tokens := []string{
		normalizeFixtureCategoryToken(name),
		normalizeFixtureCategoryToken(assetbundleName),
	}
	for _, row := range fixtureCategoryAliases {
		if normalizeFixtureCategoryToken(row.Name) != normalizeFixtureCategoryToken(name) {
			continue
		}
		for _, alias := range row.Aliases {
			tokens = append(tokens, normalizeFixtureCategoryToken(alias))
		}
		break
	}
	return tokens
}

func normalizeFixtureCategoryToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		"　", "",
		"_", "",
		"-", "",
		"・", "",
		"/", "",
	)
	return replacer.Replace(value)
}
