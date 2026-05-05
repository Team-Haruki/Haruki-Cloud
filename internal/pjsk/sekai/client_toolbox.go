package sekai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/version"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

type HarukiToolboxClient struct {
	http   *resty.Client
	config *config.ToolboxConfig
}

type MysekaiBirthdayMonitorUpsertRequest struct {
	SubscriptionID      string   `json:"subscription_id"`
	SubscriptionVersion string   `json:"subscription_version"`
	Region              string   `json:"region"`
	UID                 string   `json:"uid"`
	Materials           []string `json:"materials"`
	MaterialIDs         []int    `json:"material_ids"`
	ExpiresAt           int64    `json:"expires_at"`
	NotifyEmpty         bool     `json:"notify_empty"`
}

type MysekaiBirthdayEventLookupRequest struct {
	EventID             string `json:"event_id"`
	SubscriptionID      string `json:"subscription_id"`
	SubscriptionVersion string `json:"subscription_version"`
}

type MysekaiBirthdayEvent struct {
	EventID             string                 `json:"event_id"`
	SubscriptionID      string                 `json:"subscription_id"`
	SubscriptionVersion string                 `json:"subscription_version"`
	PayloadRef          string                 `json:"payload_ref,omitempty"`
	Region              string                 `json:"region"`
	UID                 string                 `json:"uid"`
	MatchedMaterialIDs  []int                  `json:"matched_material_ids"`
	EmptyResult         bool                   `json:"empty_result"`
	FilteredPayload     sonic.NoCopyRawMessage `json:"filtered_payload,omitempty"`
	UploadTime          int64                  `json:"upload_time"`
	CreatedAt           int64                  `json:"created_at"`
}

// NewToolboxClient constructs a HarukiToolboxClient bound to the supplied config.
// Callers own the returned client; pass it via dependency injection rather than
// reaching for a package-level singleton.
func NewToolboxClient(cfg *config.ToolboxConfig) *HarukiToolboxClient {
	return &HarukiToolboxClient{
		http:   newRestyClient().SetTimeout(apiTimeout),
		config: cfg,
	}
}

func (c *HarukiToolboxClient) userAgent() string {
	if c != nil && c.config != nil {
		if ua := strings.TrimSpace(c.config.UserAgent); ua != "" {
			return ua
		}
	}
	return version.UserAgent()
}

func (c *HarukiToolboxClient) internalRequest(ctx context.Context) (*resty.Request, error) {
	if c == nil || c.config == nil || strings.TrimSpace(c.config.BaseURL) == "" {
		return nil, ErrClientNotConfigured
	}
	req := c.http.R().
		SetContext(ctx).
		SetHeader("Authorization", c.config.APIToken).
		SetHeader("User-Agent", c.userAgent())
	return req, nil
}

func (c *HarukiToolboxClient) UpsertMysekaiBirthdayMonitor(ctx context.Context, req MysekaiBirthdayMonitorUpsertRequest) error {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/internal/mysekai-birthday-monitors/%s", strings.TrimRight(c.config.BaseURL, "/"), req.SubscriptionID)
	resp, err := r.SetBody(req).Put(endpoint)
	if err != nil {
		return fmt.Errorf("toolbox: birthday monitor upsert failed: %w", sanitizeNetworkError(err))
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: parseMessage(resp.Body())}
	}
	return nil
}

func (c *HarukiToolboxClient) DeleteMysekaiBirthdayMonitor(ctx context.Context, subscriptionID string, subscriptionVersion string) error {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/internal/mysekai-birthday-monitors/%s", strings.TrimRight(c.config.BaseURL, "/"), subscriptionID)
	resp, err := r.SetQueryParam("subscription_version", subscriptionVersion).Delete(endpoint)
	if err != nil {
		return fmt.Errorf("toolbox: birthday monitor delete failed: %w", sanitizeNetworkError(err))
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: parseMessage(resp.Body())}
	}
	return nil
}

func (c *HarukiToolboxClient) GetMysekaiBirthdayEvent(ctx context.Context, req MysekaiBirthdayEventLookupRequest) (*MysekaiBirthdayEvent, error) {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/internal/mysekai-birthday-events/%s", strings.TrimRight(c.config.BaseURL, "/"), req.EventID)
	resp, err := r.SetQueryParams(map[string]string{
		"subscription_id":      req.SubscriptionID,
		"subscription_version": req.SubscriptionVersion,
	}).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("toolbox: birthday event fetch failed: %w", sanitizeNetworkError(err))
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: parseMessage(resp.Body())}
	}
	var event MysekaiBirthdayEvent
	if err := sonic.Unmarshal(resp.Body(), &event); err != nil {
		return nil, fmt.Errorf("toolbox: failed to parse birthday event response: %w", err)
	}
	return &event, nil
}

func (c *HarukiToolboxClient) AckMysekaiBirthdayEvent(ctx context.Context, req MysekaiBirthdayEventLookupRequest) error {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/internal/mysekai-birthday-events/%s/ack", strings.TrimRight(c.config.BaseURL, "/"), req.EventID)
	resp, err := r.SetBody(req).Post(endpoint)
	if err != nil {
		return fmt.Errorf("toolbox: birthday event ack failed: %w", sanitizeNetworkError(err))
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: parseMessage(resp.Body())}
	}
	return nil
}

// GetPrivateData fetches private game-data snapshots from the Toolbox API.
//
// On success it returns the raw (decompressed) JSON body.
//
// Typed errors that callers should handle:
//   - ErrAccountBindingNotFound — user has not bound a game account on the suite
//   - ErrGameDataNotFound       — user has not uploaded any data yet
//   - ErrInvalidPlatformUser    — platform/user combo not authorised for this data
//   - ErrAccountOwnerBanned     — game account owner is banned
//   - *ToolboxAPIError          — any other unexpected non-2xx status
func (c *HarukiToolboxClient) GetPrivateData(server string, dataType ToolboxDataType, userID int64, platform, platformUserID string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("%s/api/private/game-data/%s/%s/%d", c.config.BaseURL, server, string(dataType), userID)

	resp, err := c.http.R().
		SetHeader("Authorization", c.config.APIToken).
		SetHeader("User-Agent", c.userAgent()).
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
		}).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("toolbox: request failed after retries: %w", sanitizeNetworkError(err))
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		return decompress(resp)

	case http.StatusForbidden:
		msg := parseMessage(toolboxResponseBody(resp))
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}

	case http.StatusNotFound:
		msg := parseMessage(toolboxResponseBody(resp))
		switch {
		case strings.Contains(msg, "account binding not found"):
			return nil, ErrAccountBindingNotFound
		case strings.Contains(msg, "game data not found"):
			return nil, ErrGameDataNotFound
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusNotFound, Message: msg}
		}

	case http.StatusServiceUnavailable:
		return nil, &ToolboxAPIError{StatusCode: http.StatusServiceUnavailable, Message: "toolbox service unavailable"}

	default:
		msg := parseMessage(toolboxResponseBody(resp))
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

// GetSuiteData fetches the user game-data snapshot (suite) from the Toolbox.
// The returned JSON is the raw payload equivalent to user.json, fed into userdata.Service for rendering.
func (c *HarukiToolboxClient) GetSuiteData(server string, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateData(server, ToolboxDataTypeSuite, userID, platform, platformUserID)
}

// GetMySekaiData fetches the MySekai world snapshot from the Toolbox.
// The returned JSON is the raw payload equivalent to mysekai.json, fed into the mysekai render controller.
func (c *HarukiToolboxClient) GetMySekaiData(server string, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateData(server, ToolboxDataTypeMySekai, userID, platform, platformUserID)
}

// GetPrivateDataValue queries a single top-level key from a private data snapshot.
//
// The server returns the raw value (e.g. a number or string) rather than the full JSON payload.
// This is efficient for lightweight lookups such as reading upload_time without fetching the
// entire snapshot.
//
//	GET /api/private/{server}/{dataType}/{userID}?platform=...&platform_user_id=...&key={key}
func (c *HarukiToolboxClient) GetPrivateDataValue(server string, dataType ToolboxDataType, userID int64, platform, platformUserID, key string) ([]byte, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("%s/api/private/game-data/%s/%s/%d", c.config.BaseURL, server, string(dataType), userID)

	resp, err := c.http.R().
		SetHeader("Authorization", c.config.APIToken).
		SetHeader("User-Agent", c.userAgent()).
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
			"key":              key,
		}).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("toolbox: request failed after retries: %w", sanitizeNetworkError(err))
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		return decompress(resp)
	case http.StatusForbidden:
		msg := parseMessage(toolboxResponseBody(resp))
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}
	case http.StatusNotFound:
		msg := parseMessage(toolboxResponseBody(resp))
		switch {
		case strings.Contains(msg, "account binding not found"):
			return nil, ErrAccountBindingNotFound
		case strings.Contains(msg, "game data not found"):
			return nil, ErrGameDataNotFound
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusNotFound, Message: msg}
		}
	case http.StatusServiceUnavailable:
		return nil, &ToolboxAPIError{StatusCode: http.StatusServiceUnavailable, Message: "toolbox service unavailable"}
	default:
		msg := parseMessage(toolboxResponseBody(resp))
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

// GetPrivateDataValues queries multiple top-level keys from a private data snapshot.
//
// The server returns a JSON object mapping each key to its value.
// Multiple keys are joined with commas: e.g. key=upload_time,version
func (c *HarukiToolboxClient) GetPrivateDataValues(server string, dataType ToolboxDataType, userID int64, platform, platformUserID string, keys ...string) ([]byte, error) {
	return c.GetPrivateDataValue(server, dataType, userID, platform, platformUserID, strings.Join(keys, ","))
}

// GetUploadTime fetches the upload_time timestamp (seconds) for a given data type snapshot.
// Returns the raw bytes of the integer value, e.g. []byte("1774339266").
func (c *HarukiToolboxClient) GetUploadTime(server string, dataType ToolboxDataType, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateDataValue(server, dataType, userID, platform, platformUserID, "upload_time")
}

// UserGameBinding represents a single game account binding returned by the toolbox
// fast-verification endpoint.
type UserGameBinding struct {
	Server     string `json:"server"`
	GameUserID string `json:"gameUserId"`
}

// GetToolboxUserFastVerificationGameAccountBindings returns all game account bindings
// associated with the given platform identity via the fast-verification path.
//
// The endpoint queries authorize_social_platform_infos where
// allow_fast_verification=true for the supplied platform/platform_user_id, then
// returns a deduplicated flat list of every game account bound to those users.
//
// Return value semantics:
//
//   - Empty slice (no error) — platform/user found but no associated game bindings exist.
//
//   - ErrAccountOwnerBanned  — the platform user's associated toolbox account is banned (HTTP 403).
//     This is distinct from "no bindings": a banned user returns 403, not an empty list.
//
//   - ErrInvalidPlatformUser — platform/user combo not found or not authorised (HTTP 403).
//
//   - *ToolboxAPIError       — any other unexpected non-2xx status.
//
//     GET /api/private/game-binding?platform=...&platform_user_id=...
func (c *HarukiToolboxClient) GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID string) ([]UserGameBinding, error) {
	if c == nil {
		return nil, ErrClientNotConfigured
	}
	url := fmt.Sprintf("%s/api/private/game-binding", c.config.BaseURL)

	resp, err := c.http.R().
		SetHeader("Authorization", c.config.APIToken).
		SetHeader("User-Agent", c.userAgent()).
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
		}).
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("toolbox: request failed after retries: %w", sanitizeNetworkError(err))
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		var bindings []UserGameBinding
		body, err := decompress(resp)
		if err != nil {
			return nil, err
		}
		if err := sonic.Unmarshal(body, &bindings); err != nil {
			return nil, fmt.Errorf("toolbox: failed to parse game bindings response: %w", err)
		}
		return bindings, nil

	case http.StatusForbidden:
		msg := parseMessage(toolboxResponseBody(resp))
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}

	case http.StatusNotFound:
		msg := parseMessage(toolboxResponseBody(resp))
		if strings.Contains(msg, "account binding not found") {
			return nil, ErrAccountBindingNotFound
		}
		return nil, &ToolboxAPIError{StatusCode: http.StatusNotFound, Message: msg}

	case http.StatusServiceUnavailable:
		return nil, &ToolboxAPIError{StatusCode: http.StatusServiceUnavailable, Message: "toolbox service unavailable"}

	default:
		msg := parseMessage(toolboxResponseBody(resp))
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

func toolboxResponseBody(resp *resty.Response) []byte {
	body, err := decompress(resp)
	if err != nil {
		return resp.Body()
	}
	return body
}

// decompress handles transparent zstd decompression when the server indicates it.
func decompress(resp *resty.Response) ([]byte, error) {
	body := resp.Body()
	if resp.Header().Get("Content-Encoding") != "zstd" {
		return body, nil
	}

	decoder, err := zstd.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd reader init failed: %w", err)
	}
	defer decoder.Close()

	out, err := io.ReadAll(decoder)
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd decompression failed: %w", err)
	}
	return out, nil
}
