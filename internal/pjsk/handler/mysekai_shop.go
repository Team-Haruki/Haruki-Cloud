package handler

import (
	"haruki-cloud/internal/onebot11"
	"strings"
)

func parseMysekaiShopArgs(args string) (map[string]any, error) {
	params := map[string]any{}
	shopType := ""
	for _, token := range strings.Fields(strings.ToLower(args)) {
		value := ""
		switch token {
		case "全部", "full", "all":
			params["show_all"] = true
			continue
		case "蓝图", "blueprint":
			value = "blueprint"
		case "工具", "tool":
			value = "tool"
		case "素材", "材料", "material":
			value = "material"
		default:
			return nil, onebot11.NewReplayError("未识别的商店参数：%s\n用法：/烤森商店 [蓝图/工具/材料] [全部]", token)
		}
		if shopType != "" && shopType != value {
			return nil, onebot11.NewReplayError("一次只能选择一种商品类型，可与“全部”组合")
		}
		shopType = value
	}
	if shopType != "" {
		params["shop_type"] = shopType
	}
	return params, nil
}
