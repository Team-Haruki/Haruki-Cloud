package pjsk

import (
	"strings"

	"haruki-cloud/utils/usererror"
)

const genericClientErrorText = "请求处理失败，请稍后再试"

type paramEchoRedaction struct {
	prefix      string
	replacement string
}

var paramEchoRedactions = []paramEchoRedaction{
	{prefix: "活动查询参数错误"},
	{prefix: "查卡池参数错误"},
	{prefix: "无效的参数"},
	{prefix: "无法解析的指令"},
	{prefix: "无法解析的列表查询指令"},
	{prefix: "无法解析的活动指令"},
	{prefix: "无法识别的指令格式"},
	{prefix: "无法解析的排名参数"},
	{prefix: "无法解析要比较的歌曲"},
	{prefix: "解析综合力失败"},
	{prefix: "解析活动加成失败"},
	{prefix: "解析游玩间隔失败"},
	{prefix: "无效的游戏ID"},
	{prefix: "无效的用户ID"},
	{prefix: "无效的UID"},
	{prefix: "无法识别的个人信息背景参数"},
	{prefix: "未知的查询模式"},
	{prefix: "不支持的服务器"},
	{prefix: "找不到歌曲或参数错误"},
	{prefix: "未识别到角色"},
	{prefix: "未找到角色"},
	{prefix: "匹配到多个角色"},
	{prefix: "未找到家具类别", replacement: "未找到对应的家具类别"},
	{prefix: cardNotFoundMessage},
	{prefix: "找不到特定的卡池"},
	{prefix: "event_id is required", replacement: "当前没有可推断的活动，请指定活动ID"},
	{prefix: "event id is required", replacement: "请提供活动ID"},
	{prefix: "gacha id is required", replacement: "请提供要查询的卡池"},
	{prefix: "unable to parse music query", replacement: musicQueryParseFailedMessage},
	{prefix: "invalid token", replacement: "无效的参数"},
	{prefix: "invalid event gacha query", replacement: "卡池查询参数错误"},
	{prefix: "invalid gacha id", replacement: "卡池查询参数错误"},
	{prefix: "failed to search card", replacement: cardNotFoundMessage},
	{prefix: "failed to search card list", replacement: cardNotFoundMessage},
	{prefix: "failed to search card box", replacement: cardNotFoundMessage},
	{prefix: "failed to resolve deck music selection", replacement: musicQueryParseFailedMessage},
	{prefix: "failed to resolve compare music selection", replacement: "无法解析要比较的歌曲"},
	{prefix: "failed to resolve music meta query", replacement: musicQueryParseFailedMessage},
	{prefix: "toolbox api error", replacement: "工具箱请求失败，请稍后再试"},
	{prefix: "toolbox: request failed after retries", replacement: "连接工具箱超时或网络异常，请稍后再试"},
	{prefix: "sekai api error", replacement: "SekaiAPI 拉取失败，请稍后再试"},
	{prefix: "sekaiapi profile fetch failed", replacement: "SekaiAPI 拉取失败，请稍后再试"},
	{prefix: "tracker api error", replacement: "查榜请求失败，请稍后再试"},
	{prefix: "tracker: request failed after retries", replacement: "连接查榜服务超时或网络异常，请稍后再试"},
	{prefix: "deck-service returned http", replacement: "组卡服务请求失败，请稍后再试"},
	{prefix: "deck-service unavailable", replacement: deckServiceUnavailableMessage},
	{prefix: "deck-service upstream is unavailable", replacement: deckServiceUnavailableMessage},
	{prefix: "fixed_characters and fixed_cards cannot be used together", replacement: "组卡服务版本过旧，暂不支持同时固定角色和卡牌，请更新组卡服务后重试"},
	{prefix: "获取自制谱面 JSON 失败", replacement: "获取自制谱面数据失败，请稍后再试"},
	{prefix: "获取自定义谱面信息失败", replacement: "获取自制谱面数据失败，请稍后再试"},
	{prefix: "music not found", replacement: musicNotFoundMessage},
	{prefix: "card not found (filter)", replacement: cardNotFoundMessage},
	{prefix: "no cards found for filter", replacement: cardNotFoundMessage},
	{prefix: "gacha not found", replacement: "找不到特定的卡池"},
	{prefix: "card service unavailable", replacement: "卡牌服务未就绪，请稍后再试"},
	{prefix: "music controller is not configured", replacement: musicServiceUnavailableMessage},
	{prefix: "music service unavailable", replacement: musicServiceUnavailableMessage},
	{prefix: "music data source is not configured", replacement: musicServiceUnavailableMessage},
	{prefix: "no music data source for region", replacement: musicServiceUnavailableMessage},
	{prefix: "music board service unavailable", replacement: "歌曲排行服务未就绪，请稍后再试"},
	{prefix: "music board request has no items", replacement: "歌曲排行参数错误"},
	{prefix: "music meta request is empty", replacement: "请输入要查询的歌曲名或ID"},
	{prefix: "score music service unavailable", replacement: "控分服务未就绪，请稍后再试"},
	{prefix: "invalid score control request", replacement: "控分参数错误"},
	{prefix: "invalid custom-room score request", replacement: "自定义房间控分参数错误"},
	{prefix: "misc birthday service unavailable", replacement: "生日服务未就绪，请稍后再试"},
	{prefix: "invalid birthday request", replacement: "生日查询参数错误"},
	{prefix: "query birthday characters failed", replacement: "查询生日角色失败，请稍后再试"},
	{prefix: "query birthday cards failed", replacement: "查询生日卡牌失败，请稍后再试"},
	{prefix: "event service unavailable", replacement: "活动服务未就绪，请稍后再试"},
	{prefix: "gacha service unavailable", replacement: "卡池服务未就绪，请稍后再试"},
	{prefix: "stamp service unavailable", replacement: "表情服务未就绪，请稍后再试"},
	{prefix: "vlive service unavailable", replacement: "虚拟 Live 服务未就绪，请稍后再试"},
	{prefix: "profile service unavailable", replacement: "个人信息服务未就绪，请稍后再试"},
	{prefix: "mysekai service unavailable", replacement: "烤森服务未就绪，请稍后再试"},
	{prefix: "sk service unavailable", replacement: "查榜服务未就绪，请稍后再试"},
	{prefix: "deck music resolve requires music controller", replacement: deckServiceUnavailableMessage},
	{prefix: "drawing client is not configured", replacement: "渲染服务未就绪，请稍后再试"},
	{prefix: "Post \"http://haruki-drawing", replacement: "连接渲染服务超时或网络异常，请稍后再试"},
	{prefix: "context deadline exceeded", replacement: "连接上游服务超时或网络异常，请稍后再试"},
	{prefix: "Client.Timeout exceeded", replacement: "连接上游服务超时或网络异常，请稍后再试"},
	{prefix: "api request failed with status", replacement: "渲染请求失败，请稍后再试"},
	{prefix: "image storage is not configured", replacement: "图片服务未就绪，请稍后再试"},
	{prefix: "asset path is empty", replacement: "图片资源不可用"},
}

var paramEchoSeparatorMarkers = []string{
	"参数错误",
	"无法解析",
	"无法识别",
	"无效的",
	"未知的查询模式",
	"不支持的服务器",
	"unable to parse",
	"failed to resolve",
}

func clientErrorText(message string, enableParamEcho bool) string {
	return clientErrorTextWithHelpTrigger(message, enableParamEcho, "", "")
}

func clientErrorTextForCommand(message string, enableParamEcho bool, helpTrigger, commandPath string) string {
	return clientErrorTextWithHelpTrigger(message, enableParamEcho, helpTrigger, commandPath)
}

func clientErrorTextWithHelpTrigger(message string, enableParamEcho bool, helpTrigger, commandPath string) string {
	_ = enableParamEcho
	redacted := redactParamEcho(message, helpTrigger, commandPath)
	if usererror.MessageContainsSensitiveURL(redacted) {
		return genericClientErrorText
	}
	return redacted
}

func redactParamEcho(message string, helpTrigger, commandPath string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	lines := strings.Split(message, "\n")
	if guidance, ok := parameterGuidanceForError(lines[0], commandPath); ok {
		return formatParameterGuidance(guidance, parameterHelpTrigger(lines, helpTrigger))
	}
	if replacement, ok := redactParamEchoLine(lines[0]); ok {
		if replacement == genericClientErrorText {
			return replacement
		}
		if guidance, isParameterError := parameterGuidanceForError(replacement, commandPath); isParameterError {
			return formatParameterGuidance(guidance, parameterHelpTrigger(lines, helpTrigger))
		}
		lines[0] = replacement
	}
	if compacted, ok := compactUsageHelpLines(lines, helpTrigger, commandPath); ok {
		return compacted
	}
	return strings.Join(lines, "\n")
}

func parameterHelpTrigger(lines []string, fallback string) string {
	if trigger := normalizeUsageHelpTrigger(fallback); trigger != "" {
		return trigger
	}
	for _, line := range lines {
		if trigger := extractUsageHelpTrigger(strings.TrimSpace(line)); trigger != "" {
			return trigger
		}
	}
	return ""
}

func redactParamEchoLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return line, false
	}

	for _, redaction := range paramEchoRedactions {
		if !lineHasParamEchoPrefix(line, redaction.prefix) {
			continue
		}
		if redaction.replacement != "" {
			return redaction.replacement, true
		}
		return redaction.prefix, true
	}

	for _, marker := range paramEchoSeparatorMarkers {
		if prefix, ok := redactParamEchoAfterMarker(line, marker); ok {
			return prefix, true
		}
	}

	if idx := strings.Index(line, "找不到特定的歌:"); idx >= 0 {
		return strings.TrimSpace(line[:idx+len(musicNotFoundMessage)]), true
	}
	if idx := strings.Index(line, "找不到特定的歌："); idx >= 0 {
		return strings.TrimSpace(line[:idx+len(musicNotFoundMessage)]), true
	}
	if idx := strings.Index(line, "找不到特定的卡牌:"); idx >= 0 {
		return strings.TrimSpace(line[:idx+len(cardNotFoundMessage)]), true
	}
	if idx := strings.Index(line, "找不到特定的卡牌："); idx >= 0 {
		return strings.TrimSpace(line[:idx+len(cardNotFoundMessage)]), true
	}
	if isEnglishLikeErrorLine(line) {
		return genericClientErrorText, true
	}
	return line, false
}

func compactUsageHelpLines(lines []string, fallbackTrigger, commandPath string) (string, bool) {
	if len(lines) == 0 {
		return "", false
	}
	first := strings.TrimSpace(lines[0])
	if first == "" {
		return "", false
	}
	trigger := ""
	if trigger = extractInlineUsageHelpTrigger(first); trigger != "" && len(lines) == 1 {
		if guidance, exists := parameterGuidanceForPath(commandPath); exists {
			return formatParameterGuidance(guidance, trigger), true
		}
		return "参数格式不正确\n查看完整用法请发送：" + trigger + " -help", true
	}
	trigger, hasUsageHelp := scanUsageHelpLines(lines[1:], isUsageHelpMarker(first))
	if trigger == "" {
		trigger = normalizeUsageHelpTrigger(fallbackTrigger)
	}
	if trigger == "" || !hasUsageHelp {
		return "", false
	}
	if guidance, exists := parameterGuidanceForPath(commandPath); exists {
		return formatParameterGuidance(guidance, trigger), true
	}
	if isUsageHelpMarker(first) {
		first = "参数格式不正确"
	}
	return first + "\n查看完整用法请发送：" + trigger + " -help", true
}

func scanUsageHelpLines(lines []string, hasUsageHelp bool) (string, bool) {
	trigger := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trigger == "" {
			trigger = extractUsageHelpTrigger(trimmed)
		}
		hasUsageHelp = hasUsageHelp || isUsageHelpMarker(trimmed)
	}
	return trigger, hasUsageHelp
}

func isUsageHelpMarker(line string) bool {
	line = strings.TrimSpace(line)
	return line == "使用方式" || line == "使用方式:" || line == "使用方式：" ||
		strings.HasPrefix(line, "使用方式:") || strings.HasPrefix(line, "使用方式：") ||
		strings.HasPrefix(line, "查看完整用法:") || strings.HasPrefix(line, "查看完整用法：") ||
		strings.HasPrefix(line, "查看完整用法请发送:") || strings.HasPrefix(line, "查看完整用法请发送：") ||
		strings.HasPrefix(line, "【") && strings.Contains(line, "格式】")
}

func extractUsageHelpTrigger(line string) string {
	if trigger := extractInlineUsageHelpTrigger(line); trigger != "" {
		return trigger
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "或 ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "或 "))
	}
	if strings.HasPrefix(line, "/") {
		return normalizeUsageHelpTrigger(line)
	}
	return ""
}

func extractInlineUsageHelpTrigger(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{
		"使用方式:", "使用方式：",
		"查看完整用法:", "查看完整用法：",
		"查看完整用法请发送:", "查看完整用法请发送：",
	} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return normalizeUsageHelpTrigger(rest)
	}
	return ""
}

func normalizeUsageHelpTrigger(trigger string) string {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" || !strings.HasPrefix(trigger, "/") {
		return ""
	}
	fields := strings.Fields(trigger)
	if len(fields) > 0 {
		return fields[0]
	}
	return trigger
}

func lineHasParamEchoPrefix(line, prefix string) bool {
	return line == prefix ||
		strings.HasPrefix(line, prefix+":") ||
		strings.HasPrefix(line, prefix+"：") ||
		strings.HasPrefix(line, prefix+" ")
}

func redactParamEchoAfterMarker(line, marker string) (string, bool) {
	markerIdx := strings.Index(line, marker)
	if markerIdx < 0 {
		return "", false
	}
	for _, sep := range []string{":", "："} {
		if sepIdx := strings.Index(line[markerIdx+len(marker):], sep); sepIdx >= 0 {
			sepIdx += markerIdx + len(marker)
			return strings.TrimSpace(line[:sepIdx]), true
		}
	}
	return "", false
}

func isEnglishLikeErrorLine(line string) bool {
	hasASCIIAlpha := false
	for _, r := range line {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			hasASCIIAlpha = true
		case r >= '\u4e00' && r <= '\u9fff':
			return false
		}
	}
	return hasASCIIAlpha
}
