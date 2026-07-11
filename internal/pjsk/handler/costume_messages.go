package handler

import (
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
)

func normalizeCostume3DError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case message == "3d preview service is not configured":
		return onebot11.NewReplayError("当前 Cloud 未开启3D功能")
	case strings.HasPrefix(message, "3d preview engine is not configured for region "):
		region := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(message, "3d preview engine is not configured for region ")))
		return onebot11.NewReplayError("%s服暂未配置3D渲染服务", region)
	case strings.HasPrefix(message, "3d combo role not found"):
		return onebot11.NewReplayError("这些3D部件不属于同一个角色，请检查区服、角色组合和部件ID")
	case strings.HasPrefix(message, "3d combo matches multiple roles"):
		return onebot11.NewReplayError("这些部件可匹配多个角色版本，请补充角色组合（例如 ln、mmj、vbs、ws、n25）")
	case strings.HasPrefix(message, "3d combo anchor part not found"):
		return onebot11.NewReplayError("找不到可用于确定角色的3D部件，请检查部件ID")
	case strings.HasPrefix(message, "3d combo body part not usable"):
		return onebot11.NewReplayError("服装ID %d 不能用于当前角色或组合", trailingCostume3DID(message))
	case strings.HasPrefix(message, "3d combo hair part not usable"):
		return onebot11.NewReplayError("发型ID %d 不能用于当前角色或组合", trailingCostume3DID(message))
	case strings.HasPrefix(message, "3d combo head/accessory part not usable"):
		return onebot11.NewReplayError("饰品ID %d 不能用于当前角色或组合", trailingCostume3DID(message))
	case strings.Contains(message, "head/hair combination is blocked"):
		return onebot11.NewReplayError("该饰品与发型不兼容，请更换其中一个部件")
	case strings.HasPrefix(message, "3d preview part is missing runtime package"):
		return onebot11.NewReplayError("该3D部件尚未导出完成，请更换部件或稍后再试")
	case strings.HasPrefix(message, "3d preview part not found"):
		return onebot11.NewReplayError("找不到这个3D部件ID，请检查区服和ID")
	case strings.HasPrefix(message, "3d preview default role not found"):
		return onebot11.NewReplayError("找不到该角色的默认3D模型")
	case strings.Contains(message, "tuple incomplete"):
		return onebot11.NewReplayError("该角色缺少完整的服装、发型或饰品数据，暂时无法渲染")
	case strings.HasPrefix(message, "3d preview capture is busy"):
		return onebot11.NewReplayError("3D渲染任务繁忙，请稍后再试")
	case strings.HasPrefix(message, "3d preview registry"):
		return onebot11.NewReplayError("读取3D部件数据失败，请稍后再试")
	case strings.HasPrefix(message, "3d preview capture fetch"):
		return onebot11.NewReplayError("获取3D渲染图片失败，请稍后再试")
	case strings.HasPrefix(message, "3d preview capture"):
		return onebot11.NewReplayError("3D模型渲染失败，请稍后再试")
	case strings.HasPrefix(message, "3d preview"), strings.HasPrefix(message, "3d combo"):
		return onebot11.NewReplayError("3D处理失败，请稍后再试")
	default:
		return err
	}
}

func trailingCostume3DID(message string) int {
	value := strings.TrimSpace(message[strings.LastIndex(message, ":")+1:])
	id, _ := strconv.Atoi(value)
	return id
}
