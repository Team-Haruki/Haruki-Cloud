package drawing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	hashstructure "github.com/mitchellh/hashstructure/v2"
)

const (
	renderCacheFileExt    = "png"
	renderCachePublic     = "public"
	renderCacheKeyVersion = 2
)

type RenderCacheConfig struct {
	BaseURL    string
	StorageDir string
	TTL        time.Duration
}

type RenderCacheClient struct {
	http       *resty.Client
	baseURL    string
	storageDir string
	ttl        time.Duration
}

type renderCacheRecord struct {
	Key       string `json:"key"`
	FilePath  string `json:"file_path"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

type renderCacheAPIError struct {
	Error string `json:"error"`
}

type renderCacheEndpoint struct {
	Path       string
	Query      neturl.Values
	Normalized string
}

type renderCachePolicy struct {
	Endpoint string
	APIPath  string
	UserID   string
	Params   any
}

type renderCacheKeyMaterial struct {
	Version  int    `json:"version"`
	Endpoint string `json:"endpoint"`
	APIPath  string `json:"api_path"`
	UserID   string `json:"user_id"`
	Params   any    `json:"params,omitempty"`
}

func NewRenderCacheClient(cfg RenderCacheConfig) *RenderCacheClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	storageDir := strings.TrimSpace(cfg.StorageDir)
	if baseURL == "" || storageDir == "" || cfg.TTL <= 0 {
		return nil
	}

	return &RenderCacheClient{
		http:       resty.New().SetTimeout(10 * time.Second),
		baseURL:    baseURL,
		storageDir: storageDir,
		ttl:        cfg.TTL,
	}
}

func (c *RenderCacheClient) Render(endpoint string, request interface{}, render func() ([]byte, error)) ([]byte, error) {
	if c == nil {
		return render()
	}

	policy, policyErr := buildRenderCachePolicy(endpoint, request)
	if policyErr == nil {
		key, keyErr := buildRenderCacheKey(policy)
		if keyErr == nil {
			if cached, hit := c.lookup(key); hit {
				return cached, nil
			}
		}

		image, err := render()
		if err != nil {
			return nil, err
		}

		if key, keyErr := buildRenderCacheKey(policy); keyErr == nil {
			_ = c.store(key, policy.APIPath, policy.UserID, image)
		}
		return image, nil
	}

	return render()
}

func (c *RenderCacheClient) lookup(key string) ([]byte, bool) {
	var record renderCacheRecord
	var apiErr renderCacheAPIError

	resp, err := c.http.R().
		SetQueryParam("key", key).
		SetResult(&record).
		SetError(&apiErr).
		Get(c.baseURL + "/cache")
	if err != nil {
		return nil, false
	}
	if resp.StatusCode() != http.StatusOK || strings.TrimSpace(record.FilePath) == "" {
		return nil, false
	}

	body, err := os.ReadFile(record.FilePath)
	if err != nil {
		return nil, false
	}
	return body, true
}

func (c *RenderCacheClient) store(key string, apiPath string, userID string, image []byte) error {
	targetPath := c.defaultFilePath(apiPath, userID, key)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(targetPath, image, 0o644); err != nil {
		return err
	}

	var apiErr renderCacheAPIError
	resp, err := c.http.R().
		SetFormData(map[string]string{
			"key":       key,
			"ttl":       strconv.Itoa(int(math.Ceil(c.ttl.Seconds()))),
			"api_path":  apiPath,
			"user_id":   userID,
			"ext":       renderCacheFileExt,
			"file_path": targetPath,
		}).
		SetError(&apiErr).
		Post(c.baseURL + "/cache")
	if err != nil || resp.StatusCode() != http.StatusOK {
		_ = os.Remove(targetPath)
		if err != nil {
			return err
		}
		return fmt.Errorf("cache register failed with status: %d", resp.StatusCode())
	}
	return nil
}

func (c *RenderCacheClient) defaultFilePath(apiPath string, userID string, key string) string {
	return filepath.Join(c.storageDir, filepath.FromSlash(apiPath), userID, key+"."+renderCacheFileExt)
}

func buildRenderCachePolicy(endpoint string, request interface{}) (renderCachePolicy, error) {
	parsedEndpoint, err := parseRenderCacheEndpoint(endpoint)
	if err != nil {
		return renderCachePolicy{}, err
	}

	payload, err := normalizeRenderCachePayload(request)
	if err != nil {
		return renderCachePolicy{}, err
	}
	sanitizeRenderCachePayload(parsedEndpoint.Path, payload)

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
	}, nil
}

func buildRenderCacheKey(policy renderCachePolicy) (string, error) {
	hashValue, err := hashstructure.Hash(renderCacheKeyMaterial{
		Version:  renderCacheKeyVersion,
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

func normalizeRenderCachePayload(request interface{}) (any, error) {
	if request == nil {
		return nil, nil
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal render cache payload: %w", err)
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode render cache payload: %w", err)
	}
	return payload, nil
}

func sanitizeRenderCachePayload(endpointPath string, payload any) {
	switch endpointPath {
	case "/api/pjsk/deck/recommend":
		deleteKeyAt(payload, "model_name")
		deleteKeyAt(payload, "cost_times")
		deleteKeyAt(payload, "wait_times")
		deleteKeyAt(payload, "profile", "update_time")
	case "/api/pjsk/event/record":
		deleteKeyAt(payload, "user_info", "update_time")
	case "/api/pjsk/music/list":
		deleteKeyAt(payload, "profile", "update_time")
	case "/api/pjsk/music/progress", "/api/pjsk/music/rewards/detail", "/api/pjsk/music/rewards/basic",
		"/api/pjsk/mysekai/resource", "/api/pjsk/mysekai/fixture-list", "/api/pjsk/mysekai/door-upgrade",
		"/api/pjsk/mysekai/music-record", "/api/pjsk/mysekai/talk-list":
		deleteDataSourceUpdateTimes(payload, "profile")
	case "/api/pjsk/education/challenge-live", "/api/pjsk/education/power-bonus",
		"/api/pjsk/education/area-item", "/api/pjsk/education/bonds", "/api/pjsk/education/leader-count":
		deleteKeyAt(payload, "profile", "update_time")
		deleteDataSourceUpdateTimes(payload, "profile")
	case "/api/pjsk/sk/check-room":
		deleteKeyAt(payload, "update_at")
	case "/api/pjsk/sk/winrate":
		deleteKeyAt(payload, "updated_at")
	}
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

func appendIfPresent(parts []string, key string, value string) []string {
	normalized := sanitizeScopeValue(value)
	if normalized == "" {
		return parts
	}
	return append(parts, key+"="+normalized)
}

func appendDigestIfPresent(parts []string, key string, value any) []string {
	digest := renderCacheShortDigest(value)
	if digest == "" {
		return parts
	}
	return append(parts, key+"="+digest)
}

func sanitizeScopeValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func renderCacheShortDigest(value any) string {
	if value == nil {
		return ""
	}

	hashValue, err := hashstructure.Hash(value, hashstructure.FormatV2, nil)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256([]byte(strconv.FormatUint(hashValue, 10)))
	return hex.EncodeToString(digest[:8])
}

func deleteKeyAt(root any, path ...string) {
	if len(path) == 0 {
		return
	}

	parent := mapAt(root, path[:len(path)-1]...)
	if parent == nil {
		return
	}
	delete(parent, path[len(path)-1])
}

func deleteDataSourceUpdateTimes(root any, path ...string) {
	node := mapAt(root, path...)
	if node == nil {
		return
	}
	for _, item := range sliceAt(node, "data_sources") {
		delete(mapAt(item), "update_time")
	}
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

func stringField(root any, path ...string) string {
	return strings.TrimSpace(scalarString(valueAt(root, path...)))
}

func collectFieldList(root any, listKey string, field string) []any {
	var items []any
	if listKey == "" {
		items = sliceAt(root)
	} else {
		items = sliceAt(root, listKey)
	}
	if len(items) == 0 {
		return nil
	}

	values := make([]any, 0, len(items))
	for _, item := range items {
		if field == "" {
			values = append(values, item)
			continue
		}
		value := valueAt(item, field)
		if value != nil {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func collectNestedFieldList(root any, listKey string, objectKey string, field string) []any {
	items := sliceAt(root, listKey)
	if len(items) == 0 {
		return nil
	}

	values := make([]any, 0, len(items))
	for _, item := range items {
		value := valueAt(item, objectKey, field)
		if value != nil {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func mapSubset(root any, keys ...string) map[string]any {
	source := mapAt(root)
	if source == nil {
		return nil
	}

	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := source[key]; ok {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

func boolQueryScopeValue(query neturl.Values, key string) string {
	value := strings.ToLower(strings.TrimSpace(query.Get(key)))
	if value == "" {
		return ""
	}
	if value == "true" || value == "1" {
		return "1"
	}
	return "0"
}

func boolValueString(value any) string {
	switch typed := value.(type) {
	case bool:
		if typed {
			return "1"
		}
		return "0"
	case string:
		return boolQueryScopeValue(neturl.Values{"value": {typed}}, "value")
	default:
		return ""
	}
}

func mysekaiMusicRecordShowID(payload any) string {
	for _, category := range sliceAt(payload, "category_musicrecords") {
		for _, record := range sliceAt(category, "musicrecords") {
			if valueAt(record, "id") != nil {
				return "1"
			}
		}
	}
	return "0"
}
