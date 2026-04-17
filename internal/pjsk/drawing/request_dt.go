package drawing

import (
	"context"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/displaytime"
)

const drawingTimestampMsThreshold = int64(100000000000)

func prepareDrawingRequestBody(endpoint string, body any, now time.Time, ctx context.Context) any {
	payload, err := normalizeRenderCachePayload(body)
	if err != nil {
		return body
	}

	parsed, err := parseRenderCacheEndpoint(endpoint)
	if err != nil {
		return body
	}

	root := mapAt(payload)
	if root == nil {
		return body
	}
	timeZone := scalarString(root["timezone"])
	if strings.TrimSpace(timeZone) == "" {
		timeZone = displaytime.RequestTimeZoneFromContext(ctx)
	}
	root["timezone"] = displaytime.NormalizeTimeZone(timeZone)

	nowMs := now.UnixMilli()
	if parsed.Path == "/api/pjsk/profile" {
		root["update_time"] = normalizeDrawingUpdateTime(root["update_time"], nowMs)
	}

	applyDrawingProfileDT(root, "profile", nowMs)
	applyDrawingProfileDT(root, "user_info", nowMs)
	return payload
}

func applyDrawingProfileDT(root map[string]any, key string, nowMs int64) {
	profile := mapAt(root, key)
	if profile == nil {
		return
	}

	if looksLikeDetailedProfileCard(profile) {
		profile["update_time"] = normalizeDrawingUpdateTime(profile["update_time"], nowMs)
	}

	for _, item := range sliceAt(profile, "data_sources") {
		dataSource := mapAt(item)
		if dataSource == nil {
			continue
		}
		dataSource["update_time"] = normalizeDrawingUpdateTime(dataSource["update_time"], nowMs)
	}
}

func looksLikeDetailedProfileCard(value map[string]any) bool {
	if value == nil {
		return false
	}
	if strings.TrimSpace(scalarString(value["source"])) != "" {
		return true
	}
	if value["user_cards"] != nil {
		return true
	}
	_, hasUpdateTime := value["update_time"]
	_, hasLeaderImage := value["leader_image_path"]
	return hasUpdateTime && hasLeaderImage
}

func normalizeDrawingUpdateTime(value any, fallback int64) int64 {
	parsed, ok := parseDrawingUpdateTime(value)
	if !ok || parsed <= 0 {
		return fallback
	}
	if parsed < drawingTimestampMsThreshold {
		return parsed * 1000
	}
	return parsed
}

func parseDrawingUpdateTime(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > ^uint64(0)>>1 {
			return 0, false
		}
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
