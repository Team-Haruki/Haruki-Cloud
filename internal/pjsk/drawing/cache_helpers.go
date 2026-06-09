package drawing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"math"
	neturl "net/url"
	"strconv"
	"strings"

	"github.com/mitchellh/hashstructure/v2"
)

func buildRenderCachePolicy(endpoint string, request any) (renderCachePolicy, error) {
	parsedEndpoint, err := parseRenderCacheEndpoint(endpoint)
	if err != nil {
		return renderCachePolicy{}, err
	}

	payload, err := normalizeRenderCachePayload(request)
	if err != nil {
		return renderCachePolicy{}, err
	}
	rule := sanitizeRenderCachePayload(parsedEndpoint.Path, payload)
	if !rule.Enabled {
		return renderCachePolicy{}, fmt.Errorf("render cache disabled for endpoint %s", parsedEndpoint.Path)
	}

	userID := normalizeRenderCacheUserID(extractRenderCacheUserID(payload))
	apiPath := buildRenderCacheAPIPath(parsedEndpoint, userID, payload)
	if apiPath == "" {
		return renderCachePolicy{}, fmt.Errorf("cache api path is empty")
	}

	return renderCachePolicy{
		Endpoint: parsedEndpoint.Normalized,
		APIPath:  apiPath,
		UserID:   userID,
		Params:   payload,
		TTL:      rule.TTL,
		Infinite: rule.Infinite,
	}, nil
}

func buildRenderCacheKey(policy renderCachePolicy) (string, error) {
	version := renderCacheKeyVersion
	if strings.TrimSpace(policy.APIPath) == "api/pjsk/event/list" {
		version = renderCacheEventListKeyVersion
	}
	hashValue, err := hashstructure.Hash(renderCacheKeyMaterial{
		Version:  version,
		Endpoint: policy.Endpoint,
		APIPath:  policy.APIPath,
		UserID:   policy.UserID,
		Params:   policy.Params,
	}, hashstructure.FormatV2, nil)
	if err != nil {
		return "", err
	}

	digest := sha256.Sum256([]byte(strconv.FormatUint(hashValue, 10)))
	return hex.EncodeToString(digest[:]), nil
}

func parseRenderCacheEndpoint(raw string) (renderCacheEndpoint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return renderCacheEndpoint{}, fmt.Errorf("render cache endpoint is empty")
	}

	parsed, err := neturl.Parse(trimmed)
	if err != nil {
		return renderCacheEndpoint{}, fmt.Errorf("parse render cache endpoint: %w", err)
	}
	if strings.TrimSpace(parsed.Path) == "" {
		return renderCacheEndpoint{}, fmt.Errorf("render cache endpoint path is empty")
	}

	normalized := parsed.Path
	if encoded := parsed.Query().Encode(); encoded != "" {
		normalized += "?" + encoded
	}
	return renderCacheEndpoint{
		Path:       parsed.Path,
		Query:      parsed.Query(),
		Normalized: normalized,
	}, nil
}

func normalizeRenderCachePayload(request any) (any, error) {
	if request == nil {
		return nil, nil
	}

	body, err := sonic.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal render cache payload: %w", err)
	}

	var payload any
	if err := sonic.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode render cache payload: %w", err)
	}
	return payload, nil
}

func buildRenderCacheAPIPath(endpoint renderCacheEndpoint, userID string, payload any) string {
	_ = userID
	_ = payload

	base := strings.Trim(strings.TrimSpace(endpoint.Path), "/")
	if base == "" {
		return ""
	}

	return normalizeRenderCacheAPIPath(base)
}

func extractRenderCacheUserID(payload any) string {
	if root := mapAt(payload); root != nil && isProfileLikeMap(root) {
		if candidate := extractUserIDFromMap(root); candidate != "" {
			return candidate
		}
	}
	if profile := mapAt(payload, "profile"); profile != nil {
		if candidate := extractUserIDFromMap(profile); candidate != "" {
			return candidate
		}
	}
	if userInfo := mapAt(payload, "user_info"); userInfo != nil {
		if candidate := extractUserIDFromMap(userInfo); candidate != "" {
			return candidate
		}
	}
	return ""
}

func extractUserIDFromMap(value map[string]any) string {
	if value == nil || !isProfileLikeMap(value) || isPlaceholderProfile(value) {
		return ""
	}

	if nestedProfile := mapAt(value, "profile"); nestedProfile != nil && !isPlaceholderProfile(nestedProfile) {
		if id := strings.TrimSpace(scalarString(nestedProfile["id"])); id != "" {
			return id
		}
	}

	if id := strings.TrimSpace(scalarString(value["id"])); id != "" {
		return id
	}
	return ""
}

func isProfileLikeMap(value map[string]any) bool {
	if value == nil {
		return false
	}

	if mapAt(value, "profile") != nil {
		return true
	}
	if len(sliceAt(value, "data_sources")) > 0 {
		return true
	}
	if strings.TrimSpace(scalarString(value["nickname"])) != "" {
		return true
	}
	if strings.TrimSpace(scalarString(value["leader_image_path"])) != "" {
		return true
	}
	if strings.TrimSpace(scalarString(value["source"])) != "" {
		return true
	}
	return false
}

func isPlaceholderProfile(value map[string]any) bool {
	if value == nil {
		return false
	}

	if strings.EqualFold(strings.TrimSpace(scalarString(value["id"])), "service") {
		return true
	}
	source := strings.ToLower(strings.TrimSpace(scalarString(value["source"])))
	if source == "lunabot-service" || source == "local_fallback" {
		return true
	}

	for _, item := range sliceAt(value, "data_sources") {
		source = strings.ToLower(strings.TrimSpace(scalarString(mapAt(item)["source"])))
		if source == "lunabot-service" || source == "local_fallback" {
			return true
		}
	}
	return false
}

func normalizeRenderCacheAPIPath(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func normalizeRenderCacheUserID(userID string) string {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return renderCachePublic
	}
	return trimmed
}

func valueAt(root any, path ...string) any {
	current := root
	for _, key := range path {
		next := mapAt(current)
		if next == nil {
			return nil
		}
		current = next[key]
	}
	return current
}

func mapAt(root any, path ...string) map[string]any {
	current := root
	for _, key := range path {
		next := asMap(current)
		if next == nil {
			return nil
		}
		current = next[key]
	}
	return asMap(current)
}

func asMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func sliceAt(root any, path ...string) []any {
	current := valueAt(root, path...)
	switch value := current.(type) {
	case []any:
		return value
	default:
		return nil
	}
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		if typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		if float64(typed) == math.Trunc(float64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}
