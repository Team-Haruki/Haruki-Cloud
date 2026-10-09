package handler

import (
	"strings"

	"haruki-cloud/internal/onebot11"
)

func canonicalMySekaiTrigger(mode string) string {
	switch strings.TrimSpace(mode) {
	case "mysekai-talk-list":
		return "/烤森对话列表"
	case "mysekai-fixture-detail":
		return "/msf"
	case "mysekai-map":
		return "/烤森地图"
	case "mysekai-door-upgrade":
		return "/msg"
	case "mysekai-shop":
		return "/烤森商店"
	case "mysekai-blueprint-term":
		return "/烤森限时蓝图"
	default:
		return "/mysekai"
	}
}

func normalizeMySekaiUserFacingError(err error, mode string) error {
	if err == nil {
		return nil
	}
	if isUserFacingError(err) {
		return err
	}

	message := strings.TrimSpace(err.Error())
	trigger := canonicalMySekaiTrigger(mode)

	switch {
	case strings.Contains(message, "mysekai service unavailable"),
		strings.Contains(message, "mysekai controller is not configured"),
		strings.Contains(message, "mysekai controller is not initialized"),
		strings.Contains(message, "mysekai masterdata is not configured"),
		message == "drawing client is not configured":
		return onebot11.NewReplayError("烤森服务未就绪，请稍后再试")

	case strings.HasPrefix(message, "mysekai talk list requires character query"):
		return mysekaiTalkListUsageError(trigger)

	case strings.HasPrefix(message, "mysekai talk list invalid character query:"):
		query := strings.TrimSpace(strings.TrimPrefix(message, "mysekai talk list invalid character query:"))
		if query != "" {
			return onebot11.NewReplayError("未识别到角色：%s\n使用方式:\n%s 角色名", query, trigger)
		}
		return mysekaiTalkListUsageError(trigger)

	case strings.HasPrefix(message, "mysekai map query contains no valid map ids"):
		return onebot11.NewReplayError("未识别到有效的烤森地图编号，请使用 1-4")

	case strings.HasPrefix(message, "mysekai map contains no harvest map data"):
		return onebot11.NewReplayError("当前没有可用的烤森地图数据")

	case strings.HasPrefix(message, "mysekai fixture detail invalid query:"):
		return onebot11.NewReplayError("请输入正确的家具ID\n使用方式:\n%s 家具ID", trigger)

	case strings.HasPrefix(message, "mysekai fixture detail found no valid fixtures"):
		return onebot11.NewReplayError("未找到对应的家具")

	case strings.HasPrefix(message, "mysekai fixture detail render requires exactly one fixture id"):
		return onebot11.NewReplayError("家具详情一次只能查询一个家具ID")

	case strings.HasPrefix(message, "mysekai fixture category not found:"):
		query := strings.TrimSpace(strings.TrimPrefix(message, "mysekai fixture category not found:"))
		if query == "" {
			return onebot11.NewReplayError("未找到对应的家具类别")
		}
		return onebot11.NewReplayError("未找到家具类别：%s", query)

	case strings.HasPrefix(message, "mysekai fixture category not found"):
		return onebot11.NewReplayError("未找到对应的家具类别")

	case strings.HasPrefix(message, "mysekai resource requires profile data"),
		strings.HasPrefix(message, "mysekai music record requires profile data"):
		return newMySekaiDataNotFoundReplayError()

	case strings.HasPrefix(message, "sekaiapi profile fetch failed:"),
		strings.HasPrefix(message, "sekaiapi profile build failed:"):
		return normalizeSekaiAPIFetchError(err)

	case strings.HasPrefix(message, "queried gate already max level"):
		return onebot11.NewReplayError("指定的大门已经满级")

	case strings.HasPrefix(message, "queried gate has no upgrade materials"):
		return onebot11.NewReplayError("指定的大门没有升级材料")

	case strings.HasPrefix(message, "mysekai shop snapshot missing"):
		return onebot11.NewReplayError("上传的数据缺少烤森商店信息，请重新上传完整游戏数据后再试")
	case mode == mySekaiShopCommand && strings.Contains(message, "user snapshot is not available"):
		return newMySekaiDataNotFoundReplayError()
	case strings.HasPrefix(message, "mysekai shop masterdata missing"):
		return onebot11.NewReplayError("烤森商店主数据尚未同步完整，请稍后再试")
	case strings.HasPrefix(message, "mysekai shop invalid type"):
		return onebot11.NewReplayError("商品类型请使用：蓝图、工具、材料")
	case strings.HasPrefix(message, "mysekai shop is not available in region"):
		return onebot11.NewReplayError("该区服暂未开放烤森商店")

	case strings.HasPrefix(message, "mysekai blueprint terms are not available in region"):
		return onebot11.NewReplayError("该区服暂无限时蓝图数据")

	case strings.HasPrefix(message, "mysekai blueprint terms have no current term in region"):
		return onebot11.NewReplayError("当前没有进行中或即将开始的限时蓝图\n查看全部请使用：%s all", trigger)

	case isMySekaiNewViewMode(mode) && strings.Contains(message, "drawing request failed with status 404"):
		return onebot11.NewReplayError("绘图服务暂不支持该功能，请稍后再试")

	case strings.HasPrefix(message, "decode mysekai data:"):
		detail := strings.TrimSpace(strings.TrimPrefix(message, "decode mysekai data:"))
		if detail == "" {
			return onebot11.NewReplayError("解析烤森数据失败")
		}
		return onebot11.NewReplayError("解析烤森数据失败：%s", detail)
	}

	return err
}

// isMySekaiNewViewMode reports the JP 7.0.0 views whose Drawing endpoints
// may not be deployed yet.
func isMySekaiNewViewMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "mysekai-shop", "mysekai-blueprint-term":
		return true
	default:
		return false
	}
}
