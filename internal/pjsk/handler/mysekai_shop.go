package handler

import (
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

func parseMysekaiShopArgs(args string) (map[string]any, error) {
	params := map[string]any{}
	shopType := ""
	for _, token := range strings.Fields(strings.ToLower(args)) {
		value := ""
		switch token {
		case "全部", "full", "all": //copylint:ignore 解析关键字
			params["show_all"] = true
			continue
		case "蓝图", "blueprint": //copylint:ignore 解析关键字
			value = "blueprint"
		case "工具", "tool": //copylint:ignore 解析关键字
			value = "tool"
		case "素材", "材料", "material": //copylint:ignore 解析关键字
			value = "material"
		default:
			return nil, usererror.BadParam(token, i18n.M("mysekai.shop.param_unknown"))
		}
		if shopType != "" && shopType != value {
			return nil, usererror.Invalid(i18n.M("mysekai.shop.type_once"))
		}
		shopType = value
	}
	if shopType == "" && params["show_all"] != true {
		shopType = "blueprint"
	}
	if shopType != "" {
		params["shop_type"] = shopType
	}
	return params, nil
}
