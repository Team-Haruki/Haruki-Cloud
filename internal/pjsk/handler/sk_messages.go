package handler

import (
	"errors"
	"strings"

	"haruki-cloud/internal/onebot11"
)

func normalizeSKUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "event_id is required when no current event can be inferred"):
		return onebot11.NewReplayError("当前没有可推断的活动，请指定活动ID")
	case strings.Contains(message, "tracker ranks/user_id are empty"),
		strings.Contains(message, "request has no ranks"),
		strings.Contains(message, "requires user_id or rank"):
		return onebot11.NewReplayError("请指定排名或用户ID")
	case strings.Contains(message, "region must be one of"):
		return onebot11.NewReplayError("区服参数错误，请使用 jp/cn/tw/kr/en")
	case strings.Contains(message, "wl_character_id is only valid for world bloom event"):
		return onebot11.NewReplayError("WL 章节角色只能用于 World Link 活动")
	case strings.Contains(message, "sk service unavailable"),
		strings.Contains(message, "sk controller is not initialized"),
		strings.Contains(message, "tracker client is not configured"),
		strings.Contains(message, "forecast cache is not configured"),
		strings.Contains(message, "forecast provider is not configured"),
		strings.Contains(message, "drawing client is not configured"):
		return onebot11.NewReplayError("查榜服务未就绪，请稍后再试")
	}

	if normalized := normalizeTrackerUserFacingError(err); normalized != err {
		return normalized
	}

	if normalized := normalizeDrawingUserFacingError(err); normalized != err {
		return normalized
	}
	return err
}
