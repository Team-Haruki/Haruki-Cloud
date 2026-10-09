package inventory

import (
	"strings"

	"haruki-cloud/internal/i18n"
)

type inventorySectionDef struct {
	key   string
	title i18n.Message
}

var inventorySectionOrder = []inventorySectionDef{
	{key: "currency", title: i18n.M("render_inventory.section.currency")},
	{key: "boost", title: i18n.M("render_inventory.section.boost")},
	{key: "basic", title: i18n.M("render_inventory.section.basic")},
	{key: "training", title: i18n.M("render_inventory.section.training")},
	{key: "costume", title: i18n.M("render_inventory.section.costume")},
	{key: "music", title: i18n.M("render_inventory.section.music")},
	{key: "tickets", title: i18n.M("render_inventory.section.tickets")},
	{key: "event", title: i18n.M("render_inventory.section.event")},
	{key: "memory", title: i18n.M("render_inventory.section.memory")},
	{key: "mysekai", title: i18n.M("render_inventory.section.mysekai")},
	{key: "other", title: i18n.M("render_inventory.section.misc")},
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
