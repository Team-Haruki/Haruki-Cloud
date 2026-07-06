package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type upstreamErrorPayload struct {
	Detail  string `json:"detail"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func normalizeTrackerUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	switch {
	case errors.Is(err, sekaiapi.ErrRankingNotFound):
		return onebot11.NewReplayError("当前榜单没有找到对应的排行榜数据")
	case errors.Is(err, sekaiapi.ErrServerMaintenance):
		return onebot11.NewReplayError("当前游戏服务器维护中，请稍后再试")
	}

	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "tracker client is not configured"),
		strings.Contains(lower, "tracker: base_url is empty"):
		return onebot11.NewReplayError("查榜服务未就绪，请稍后再试")
	case strings.Contains(lower, "tracker: request failed after retries"),
		strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "client.timeout exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return onebot11.NewReplayError("连接查榜服务超时或网络异常，请稍后再试")
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"):
		return onebot11.NewReplayError("查榜服务暂时不可用，请稍后再试")
	case strings.Contains(lower, "tracker: failed to unmarshal response"):
		return onebot11.NewReplayError("查榜服务返回数据解析失败，请稍后再试")
	}

	var apiErr *sekaiapi.TrackerAPIError
	if errors.As(err, &apiErr) {
		if translated, ok := translateTrackerAPIDetail(apiErr.Message); ok {
			return onebot11.NewReplayError("%s", translated)
		}
		if strings.TrimSpace(apiErr.Message) == "" {
			return onebot11.NewReplayError("查榜请求失败（状态 %d）", apiErr.StatusCode)
		}
		return onebot11.NewReplayError("查榜请求失败（状态 %d）", apiErr.StatusCode)
	}

	if strings.Contains(lower, "tracker api error:") {
		if translated, ok := translateTrackerAPIDetail(extractQuotedMessage(message)); ok {
			return onebot11.NewReplayError("%s", translated)
		}
		return onebot11.NewReplayError("查榜请求失败，请稍后再试")
	}

	return err
}

func normalizeDrawingUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	switch {
	case message == "drawing client is not configured",
		strings.Contains(lower, "drawing upstream is unavailable"),
		strings.Contains(lower, "drawing client base_url is empty"):
		return onebot11.NewReplayError("渲染服务未就绪，请稍后再试")
	case message == "image storage is not configured":
		return onebot11.NewReplayError("图片服务未就绪，请稍后再试")
	case strings.Contains(lower, "asset path is empty"):
		return onebot11.NewReplayError("图片资源不可用")
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "client.timeout exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return onebot11.NewReplayError("连接渲染服务超时或网络异常，请稍后再试")
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "eof"):
		return onebot11.NewReplayError("渲染服务暂时不可用，请稍后再试")
	case strings.HasPrefix(lower, "api request failed with status:"):
		status, detail, ok := extractStatusAndPayload(message, "api request failed with status:")
		if ok {
			if translated, ok := translateDrawingAPIDetail(detail); ok {
				return onebot11.NewReplayError("%s", translated)
			}
			switch {
			case status == 404:
				return onebot11.NewReplayError("图片资源缺失，暂时无法渲染")
			case status >= 500:
				return onebot11.NewReplayError("渲染服务内部错误，请稍后再试")
			default:
				return onebot11.NewReplayError("渲染请求失败（状态 %d）", status)
			}
		}
	}

	if translated, ok := translateDrawingAPIDetail(message); ok {
		return onebot11.NewReplayError("%s", translated)
	}

	return err
}

func normalizeSKPlayerTraceDrawingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	if isDrawingDataInsufficientError(err) {
		return onebot11.NewReplayError("玩家轨迹数据不足，暂时无法渲染")
	}
	return err
}

func isDrawingDataInsufficientError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return false
	}
	for _, candidate := range []string{message, parseEmbeddedErrorText(message)} {
		lower := strings.ToLower(strings.TrimSpace(candidate))
		switch {
		case lower == "":
			continue
		case strings.Contains(lower, "data insufficient"),
			strings.Contains(lower, "insufficient data"),
			strings.Contains(lower, "not enough data"),
			strings.Contains(lower, "数据不足"),
			strings.Contains(lower, "single positional indexer is out-of-bounds"),
			strings.Contains(lower, "out-of-bounds"),
			strings.Contains(lower, "out of bounds"),
			strings.Contains(lower, "index out of range"):
			return true
		}
	}
	return false
}

func normalizeDeckServiceUserFacingError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "deck-service unavailable"),
		strings.Contains(lower, "deck-service upstream is unavailable"),
		strings.Contains(lower, "deck-service target base_url is empty"),
		strings.Contains(lower, "deck-service target state is not initialized"),
		strings.Contains(lower, "circuit breaker open"):
		return onebot11.NewReplayError("组卡服务未就绪，请稍后再试")
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return onebot11.NewReplayError("获取组卡所需数据超时，请稍后重试")
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "eof"):
		return onebot11.NewReplayError("组卡服务暂时不可用，请稍后再试")
	}

	if translated, ok := translateDeckServiceDetail(message); ok {
		return onebot11.NewReplayError("%s", translated)
	}

	if strings.Contains(lower, "deck-service returned http ") {
		status, detail, ok := extractStatusAndPayload(message, "deck-service returned HTTP")
		if ok {
			if translated, ok := translateDeckServiceDetail(detail); ok {
				return onebot11.NewReplayError("%s", translated)
			}
			switch {
			case status >= 500:
				return onebot11.NewReplayError("组卡服务内部错误，请稍后再试")
			case status >= 400:
				return onebot11.NewReplayError("组卡请求失败（状态 %d）", status)
			}
		}
	}

	return err
}

func translateToolboxAPIDetail(dataLabel string, message string, binding *accountdata.ResolvedBinding) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return "", false
	case strings.Contains(lower, "invalid platform or platform_user_id"):
		if useTempBindingNotice() {
			return ErrMsgTempBindingUnavailable, true
		}
		return buildToolboxAccessDeniedMessage(dataLabel, binding), true
	case strings.Contains(lower, "account owner is banned"):
		return fmt.Sprintf("工具箱账号已被封禁，无法获取%s数据", dataLabel), true
	case strings.Contains(lower, "account binding not found"):
		if useTempBindingNotice() {
			return ErrMsgTempBindingUnavailable, true
		}
		return fmt.Sprintf("你还没有在工具箱绑定游戏账号，无法获取%s数据，请前往工具箱绑定游戏账号并上传数据后重试\n%s", dataLabel, ErrMsgToolboxURL), true
	case strings.Contains(lower, "game data not found"):
		return fmt.Sprintf("工具箱未找到当前%s数据，请确认已上传对应数据后重试", dataLabel), true
	case strings.Contains(lower, "toolbox service unavailable"):
		return "工具箱服务暂时不可用，请稍后再试", true
	case strings.Contains(lower, "missing token"), strings.Contains(lower, "invalid token"):
		return "工具箱鉴权失败，请检查 Cloud 与工具箱配置", true
	default:
		return "", false
	}
}

func translateSekaiAPIDetail(message string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return "", false
	case strings.Contains(lower, "missing token"):
		return "缺少访问令牌，请检查 Cloud 与 SekaiAPI 配置", true
	case strings.Contains(lower, "invalid token"):
		return "访问令牌无效，请检查 Cloud 与 SekaiAPI 配置", true
	case strings.Contains(lower, "not authorized for this server"):
		return "当前令牌无权访问该服务器", true
	case strings.Contains(lower, "invalid api type"):
		return "请求类型无效", true
	case strings.Contains(lower, "internal server error"):
		return "服务内部错误，请稍后再试", true
	case strings.Contains(lower, "upstream unavailable"):
		return "上游服务暂时不可用，请稍后再试", true
	default:
		return "", false
	}
}

func translateTrackerAPIDetail(message string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return "", false
	case strings.Contains(lower, "rate limited by tracker"):
		return "查榜请求过于频繁，请稍后再试", true
	case strings.Contains(lower, "no heartbeat found"):
		return "当前查榜服务未获取到该活动的榜线数据，请稍后再试", true
	case strings.Contains(lower, "invalid server"):
		return "查榜区服参数错误，请使用 jp/cn/tw/kr/en", true
	case strings.Contains(lower, "ranking record not found"):
		return "当前榜单没有找到对应的排行榜数据", true
	case strings.Contains(lower, "not found"):
		return "当前查榜服务未找到对应数据", true
	default:
		return "", false
	}
}

func translateDrawingAPIDetail(message string) (string, bool) {
	detail := parseEmbeddedErrorText(message)
	lower := strings.ToLower(strings.TrimSpace(detail))
	switch {
	case lower == "":
		return "", false
	case strings.Contains(lower, "content size is too large"):
		return "渲染内容过大，请减少查询内容后重试", true
	case strings.Contains(lower, "canvas size is too large"):
		return "渲染画布过大，请减少查询内容后重试", true
	case strings.Contains(lower, "target file not found"),
		strings.Contains(lower, "file not found"),
		strings.Contains(lower, "no such file or directory"),
		strings.Contains(lower, "asset path is empty"),
		strings.Contains(lower, "图片文件不存在"):
		return "图片资源缺失，暂时无法渲染", true
	case strings.Contains(lower, "cannot identify image file"),
		strings.Contains(lower, "failed to read image"),
		strings.Contains(lower, "failed to open image"):
		return "图片资源损坏，暂时无法渲染", true
	case strings.Contains(lower, "download") && strings.Contains(lower, "failed"):
		return "图片资源下载失败，请稍后再试", true
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"):
		return "渲染服务暂时不可用，请稍后再试", true
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "client.timeout exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return "连接渲染服务超时或网络异常，请稍后再试", true
	default:
		return "", false
	}
}

func translateDeckServiceDetail(message string) (string, bool) {
	detail := parseEmbeddedErrorText(message)
	lower := strings.ToLower(strings.TrimSpace(detail))
	switch {
	case lower == "":
		return "", false
	case strings.Contains(lower, "fixed_characters and fixed_cards cannot be used together"):
		return "组卡服务版本过旧，暂不支持同时固定角色和卡牌，请更新组卡服务后重试", true
	case strings.Contains(lower, "event not found for eventid:"),
		strings.Contains(lower, "master data not found"):
		return "组卡服务找不到该活动的 masterdata，请更新 masterdata 后重试", true
	case strings.Contains(lower, "music metas not found"),
		strings.Contains(lower, "music meta not found"):
		return "组卡服务找不到该区服的歌曲元数据，请更新 masterdata 后重试", true
	case strings.Contains(lower, "userdata_hash is required"),
		strings.Contains(lower, "user data not found for userdata_hash"),
		strings.Contains(lower, "cache_userdata returned empty userdata_hash"):
		return "组卡缓存数据失效，请稍后重试", true
	case strings.Contains(lower, "unsupported media type"),
		strings.Contains(lower, "unsupported content type"),
		strings.Contains(lower, "invalid content type"):
		return "组卡服务协议不兼容，请更新相关服务后重试", true
	case strings.Contains(lower, "invalid recommend payload"),
		strings.Contains(lower, "requires batch_options"):
		return "组卡请求参数错误", true
	case strings.Contains(lower, "no user data bytes available"),
		strings.Contains(lower, "user data is required"):
		return "组卡所需的用户数据无效，请重新上传数据后重试", true
	case strings.Contains(lower, "returned empty response"):
		return "组卡服务返回空结果，请稍后再试", true
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"):
		return "组卡服务暂时不可用，请稍后再试", true
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return "获取组卡所需数据超时，请稍后重试", true
	default:
		return "", false
	}
}

func extractStatusAndPayload(message string, prefix string) (int, string, bool) {
	idx := strings.Index(message, prefix)
	if idx < 0 {
		return 0, "", false
	}
	rest := strings.TrimSpace(message[idx+len(prefix):])
	status, tail, ok := consumeLeadingInt(rest)
	if !ok {
		return 0, "", false
	}
	tail = strings.TrimSpace(strings.TrimLeft(tail, " :,"))
	lowerTail := strings.ToLower(tail)
	switch {
	case strings.HasPrefix(lowerTail, "body:"):
		tail = strings.TrimSpace(tail[len("body:"):])
	case strings.HasPrefix(lowerTail, "payload:"):
		tail = strings.TrimSpace(tail[len("payload:"):])
	}
	return status, parseEmbeddedErrorText(tail), true
}

func consumeLeadingInt(value string) (int, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, "", false
	}
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, "", false
	}
	status, err := strconv.Atoi(value[:end])
	if err != nil {
		return 0, "", false
	}
	return status, value[end:], true
}

func parseEmbeddedErrorText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var quoted string
	if err := sonic.Unmarshal([]byte(raw), &quoted); err == nil && strings.TrimSpace(quoted) != "" {
		return strings.TrimSpace(quoted)
	}

	var payload upstreamErrorPayload
	if err := sonic.Unmarshal([]byte(raw), &payload); err == nil {
		for _, candidate := range []string{payload.Detail, payload.Error, payload.Message} {
			if trimmed := strings.TrimSpace(candidate); trimmed != "" {
				return trimmed
			}
		}
	}

	return raw
}

func extractQuotedMessage(message string) string {
	message = strings.TrimSpace(message)
	start := strings.IndexByte(message, '"')
	end := strings.LastIndexByte(message, '"')
	if start >= 0 && end > start {
		return strings.TrimSpace(message[start+1 : end])
	}
	return message
}
