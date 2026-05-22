package handler

import (
	"errors"
	"strings"

	"haruki-cloud/internal/onebot11"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func normalizeCardUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindCard {
		return onebot11.NewReplayError("该卡牌还未上线")
	}
	if query, ok := extractCardNotFoundQuery(err); ok {
		return onebot11.NewReplayError("找不到特定的卡牌: %s", cardNotFoundQueryOrFallback(query))
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "card ids are required"),
		strings.Contains(message, "no card query provided"):
		return onebot11.NewReplayError("请提供要查询的卡牌")
	case strings.Contains(message, "no released cards found"):
		return onebot11.NewReplayError("当前没有已上线卡牌")
	case strings.Contains(message, "card service unavailable"),
		strings.Contains(message, "card controller is not configured"),
		strings.Contains(message, "no card data source for region"),
		strings.Contains(message, "drawing client is not configured"):
		return onebot11.NewReplayError("卡牌服务未就绪，请稍后再试")
	case strings.Contains(message, "does not have original image assets"):
		return onebot11.NewReplayError("该卡牌没有可用的卡面原图")
	}
	return err
}

func extractCardNotFoundQuery(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := strings.TrimSpace(err.Error())

	const queryCardMarker = "query card "
	if idx := strings.LastIndex(message, queryCardMarker); idx >= 0 {
		raw := readNumberAfter(message[idx+len(queryCardMarker):])
		if raw != "" {
			return raw, true
		}
	}

	for _, prefix := range []string{
		"card not found (filter):",
		"no cards found for filter:",
		"card not found:",
	} {
		if idx := strings.LastIndex(message, prefix); idx >= 0 {
			raw := strings.TrimSpace(message[idx+len(prefix):])
			return cardNotFoundQueryOrFallback(raw), true
		}
	}

	switch {
	case strings.Contains(message, "card sequence out of range"),
		strings.Contains(message, "card sequence must be negative"),
		strings.Contains(message, "no cards found for character"):
		return cardNotFoundQueryOrFallback(""), true
	default:
		return "", false
	}
}

func cardNotFoundQueryOrFallback(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "未知"
	}
	return query
}

func normalizeMusicUserFacingError(err error) error {
	return normalizeMusicUserFacingErrorForLookup(err, DefaultRegionStr, "")
}

func normalizeMusicUserFacingErrorForLookup(err error, region string, fallbackQuery string) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindMusic {
		return onebot11.NewReplayError("该歌曲还未上线")
	}
	if query, ok := extractMusicNotFoundQuery(err, fallbackQuery); ok {
		return newMusicNotFoundReplayError(region, query)
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "未找到对应自定义谱面"):
		return onebot11.NewReplayError("未找到对应自定义谱面")
	case strings.Contains(message, "当前服务器暂未支持自定义谱面"):
		return onebot11.NewReplayError("当前服务器暂未支持自定义谱面请使用jp前缀查询")
	case strings.Contains(message, "music controller is not configured"),
		strings.Contains(message, "music service unavailable"),
		strings.Contains(message, "music data source is not configured"),
		strings.Contains(message, "自制谱面数据源未配置"),
		strings.Contains(message, "no music data source for region"),
		strings.Contains(message, "drawing client is not configured"):
		return onebot11.NewReplayError("歌曲服务未就绪，请稍后再试")
	case strings.Contains(message, "music ids are required"),
		strings.Contains(message, "music query is empty"),
		strings.Contains(message, "music title query is empty"),
		strings.Contains(message, "music meta request is empty"):
		return onebot11.NewReplayError("请输入要查询的歌曲名或ID")
	case strings.Contains(message, "no music matched the current filters"),
		strings.Contains(message, "no reward-eligible musics found"):
		return onebot11.NewReplayError("找不到符合条件的歌曲")
	case strings.Contains(message, "does not have jacket asset"):
		return onebot11.NewReplayError("该歌曲没有可用封面")
	case strings.Contains(message, "does not have") && strings.Contains(message, "chart"):
		return onebot11.NewReplayError("该歌曲没有对应难度谱面")
	}
	return err
}

func newMusicNotFoundReplayError(region string, query string) error {
	return onebot11.NewReplayError(
		"%s服找不到特定的歌: %s\n如果需要查其他服务器歌曲请加区服前缀",
		musicNotFoundRegionLabel(region),
		musicNotFoundQueryOrFallback(query, ""),
	)
}

func musicNotFoundRegionLabel(region string) string {
	region = strings.ToLower(strings.TrimSpace(regionWithDefault(region)))
	if region == "" {
		return DefaultRegionStr
	}
	return strings.ToUpper(region)
}

func extractMusicNotFoundQuery(err error, fallbackQuery string) (string, bool) {
	if err == nil {
		return "", false
	}
	message := strings.TrimSpace(err.Error())
	fallbackQuery = strings.TrimSpace(fallbackQuery)

	const musicNotFoundPrefix = "music not found"
	if message == musicNotFoundPrefix {
		return musicNotFoundQueryOrFallback("", fallbackQuery), true
	}
	if idx := strings.LastIndex(message, musicNotFoundPrefix+":"); idx >= 0 {
		raw := strings.TrimSpace(message[idx+len(musicNotFoundPrefix)+1:])
		return musicNotFoundQueryOrFallback(raw, fallbackQuery), true
	}

	if strings.HasPrefix(message, "music ") && strings.HasSuffix(message, " not found") {
		raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(message, "music "), " not found"))
		return musicNotFoundQueryOrFallback(raw, fallbackQuery), true
	}
	if strings.Contains(message, "music not found") {
		return musicNotFoundQueryOrFallback(extractQueryMusicIDFromError(message), fallbackQuery), true
	}
	if strings.Contains(message, "ban event index out of range") {
		return musicNotFoundQueryOrFallback("", fallbackQuery), true
	}
	if strings.Contains(message, "no ban events found for character") {
		return musicNotFoundQueryOrFallback("", fallbackQuery), true
	}
	return "", false
}

func musicNotFoundQueryOrFallback(query string, fallback string) string {
	query = strings.TrimSpace(query)
	if query == "" || strings.EqualFold(query, "empty query") {
		query = strings.TrimSpace(fallback)
	}
	if query == "" {
		return "未知"
	}
	return query
}

func extractQueryMusicIDFromError(message string) string {
	const marker = "query music "
	idx := strings.LastIndex(message, marker)
	if idx < 0 {
		return ""
	}
	return readNumberAfter(message[idx+len(marker):])
}

func readNumberAfter(rest string) string {
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	return strings.TrimSpace(rest[:end])
}

func normalizeEventUserFacingError(err error) error {
	return normalizeEventUserFacingErrorForRegion(err, DefaultRegionStr)
}

func normalizeEventUserFacingErrorForRegion(err error, region string) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindEvent {
		return onebot11.NewReplayError("该活动还未上线")
	}
	if errors.Is(err, renderevent.ErrNoOngoingEvent) {
		return onebot11.NewReplayError("当前无正在举办的活动")
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case isEventDataNotFoundMessage(message):
		return onebot11.NewReplayError("当前%s服未找到该活动数据，如需查询其他服务器活动请加对应区服前缀", musicNotFoundRegionLabel(region))
	case strings.Contains(message, "event id is required"):
		return onebot11.NewReplayError("请提供要查询的活动")
	case strings.Contains(message, "no events matched filters"):
		return onebot11.NewReplayError("找不到符合条件的活动")
	case strings.Contains(message, "event service unavailable"),
		strings.Contains(message, "no event data source for region"),
		strings.Contains(message, "drawing client is not configured"):
		return onebot11.NewReplayError("活动服务未就绪，请稍后再试")
	case strings.Contains(message, "event record requires"):
		return onebot11.NewReplayError("活动记录数据不足，无法渲染")
	}
	return err
}

func isEventDataNotFoundMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	return strings.Contains(message, "event not found") ||
		strings.Contains(message, "no events found for region")
}

func normalizeGachaUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	var unreleased *releasecheck.UnreleasedError
	if errors.As(err, &unreleased) && unreleased.Kind == releasecheck.KindGacha {
		return onebot11.NewReplayError("该卡池还未上线")
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "gacha id is required"),
		strings.Contains(message, "event id is required"):
		return onebot11.NewReplayError("请提供要查询的卡池")
	case strings.Contains(message, "gacha not found"),
		strings.Contains(message, "no gacha data matched filters"):
		return onebot11.NewReplayError("找不到特定的卡池")
	case strings.Contains(message, "gacha service unavailable"),
		strings.Contains(message, "no gacha data source for region"),
		strings.Contains(message, "drawing client is not configured"):
		return onebot11.NewReplayError("卡池服务未就绪，请稍后再试")
	}
	return err
}
