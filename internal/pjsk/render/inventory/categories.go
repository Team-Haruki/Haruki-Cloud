package inventory

import "strings"

type inventorySectionDef struct {
	key   string
	title string
}

var inventorySectionOrder = []inventorySectionDef{
	{key: "currency", title: "货币"},
	{key: "boost", title: "演出能量"},
	{key: "basic", title: "基础材料"},
	{key: "training", title: "育成材料"},
	{key: "costume", title: "服装材料"},
	{key: "music", title: "音乐与演唱"},
	{key: "tickets", title: "招募与兑换券"},
	{key: "event", title: "活动材料"},
	{key: "memory", title: "记忆"},
	{key: "mysekai", title: "MySekai 材料"},
	{key: "other", title: "其他"},
}

func inventoryCategoryForMaterial(materialType string, name string) string {
	typ := strings.ToLower(strings.TrimSpace(materialType))
	lowerName := strings.ToLower(strings.TrimSpace(name))

	switch {
	case typ == "coin" || typ == "jewel" || typ == "virtual_coin":
		return "currency"
	case strings.Contains(typ, "boost"):
		return "boost"
	case strings.Contains(typ, "costume"):
		return "costume"
	case strings.Contains(typ, "music") ||
		strings.Contains(typ, "vocal") ||
		strings.Contains(typ, "song"):
		return "music"
	case strings.Contains(typ, "ticket") ||
		strings.Contains(lowerName, "券") ||
		strings.Contains(lowerName, "ticket"):
		return "tickets"
	case strings.Contains(typ, "event") ||
		strings.Contains(lowerName, "活动") ||
		strings.Contains(lowerName, "交换所"):
		return "event"
	case strings.Contains(typ, "special_training") ||
		strings.Contains(typ, "master_lesson") ||
		strings.Contains(typ, "skill") ||
		strings.Contains(typ, "character_rank") ||
		strings.Contains(lowerName, "练习") ||
		strings.Contains(lowerName, "技能") ||
		strings.Contains(lowerName, "想法"):
		return "training"
	case typ == "" ||
		strings.Contains(typ, "material") ||
		strings.Contains(typ, "piece") ||
		strings.Contains(typ, "gem"):
		return "basic"
	default:
		return "other"
	}
}
