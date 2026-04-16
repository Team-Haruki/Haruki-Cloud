package sekai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"haruki-cloud/config"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

type HarukiToolboxClient struct {
	http   *resty.Client
	config *config.ToolboxConfig
}

const defaultToolboxUserAgent = "Haruki-Cloud/ToolboxClient"

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
	return defaultToolboxUserAgent
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
		msg := parseMessage(resp.Body())
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}

	case http.StatusNotFound:
		msg := parseMessage(resp.Body())
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
		msg := parseMessage(resp.Body())
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
		return resp.Body(), nil
	case http.StatusForbidden:
		msg := parseMessage(resp.Body())
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}
	case http.StatusNotFound:
		msg := parseMessage(resp.Body())
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
		msg := parseMessage(resp.Body())
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
		if err := sonic.Unmarshal(resp.Body(), &bindings); err != nil {
			return nil, fmt.Errorf("toolbox: failed to parse game bindings response: %w", err)
		}
		return bindings, nil

	case http.StatusForbidden:
		msg := parseMessage(resp.Body())
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}

	case http.StatusNotFound:
		msg := parseMessage(resp.Body())
		if strings.Contains(msg, "account binding not found") {
			return nil, ErrAccountBindingNotFound
		}
		return nil, &ToolboxAPIError{StatusCode: http.StatusNotFound, Message: msg}

	case http.StatusServiceUnavailable:
		return nil, &ToolboxAPIError{StatusCode: http.StatusServiceUnavailable, Message: "toolbox service unavailable"}

	default:
		msg := parseMessage(resp.Body())
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
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
