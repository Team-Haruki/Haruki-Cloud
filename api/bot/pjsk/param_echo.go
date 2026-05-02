package pjsk

import "strings"

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
	{prefix: "unable to parse music query"},
	{prefix: "invalid token"},
	{prefix: "failed to resolve compare music selection"},
	{prefix: "failed to resolve music meta query"},
	{prefix: "music not found"},
	{prefix: "card not found (filter)"},
	{prefix: "no cards found for filter"},
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
	if enableParamEcho {
		return message
	}
	return redactParamEcho(message)
}

func redactParamEcho(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	lines := strings.Split(message, "\n")
	if replacement, ok := redactParamEchoLine(lines[0]); ok {
		lines[0] = replacement
	}
	return strings.Join(lines, "\n")
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
		return strings.TrimSpace(line[:idx+len("找不到特定的歌")]), true
	}
	if idx := strings.Index(line, "找不到特定的歌："); idx >= 0 {
		return strings.TrimSpace(line[:idx+len("找不到特定的歌")]), true
	}
	return line, false
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
