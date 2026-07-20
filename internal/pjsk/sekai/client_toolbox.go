package sekai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/version"

	"github.com/bytedance/sonic"
	"github.com/go-resty/resty/v2"
	"github.com/klauspost/compress/zstd"
)

type HarukiToolboxClient struct {
	http   *resty.Client
	config *config.ToolboxConfig
}

const (
	toolboxMaxResponseBytes             = 64 << 20
	toolboxMaxDecompressedResponseBytes = 256 << 20
)

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
		http: newRestyClient().
			SetTimeout(apiTimeout).
			SetResponseBodyLimit(toolboxMaxResponseBytes),
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
	if ctx == nil {
		ctx = context.TODO()
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
	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := r.SetBody(req).Put(endpoint)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
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
	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := r.SetQueryParam("subscription_version", subscriptionVersion).Delete(endpoint)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
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
	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := r.SetQueryParams(map[string]string{
		"subscription_id":      req.SubscriptionID,
		"subscription_version": req.SubscriptionVersion,
	}).Get(endpoint)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("toolbox: birthday event fetch failed: %w", sanitizeNetworkError(err))
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: parseMessage(resp.Body())}
	}
	var event MysekaiBirthdayEvent
	finishDecode := commandtrace.MeasureOperation(ctx, "toolbox.decode")
	decodeErr := sonic.Unmarshal(resp.Body(), &event)
	finishDecode()
	if decodeErr != nil {
		return nil, fmt.Errorf("toolbox: failed to parse birthday event response: %w", decodeErr)
	}
	return &event, nil
}

func (c *HarukiToolboxClient) AckMysekaiBirthdayEvent(ctx context.Context, req MysekaiBirthdayEventLookupRequest) error {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/internal/mysekai-birthday-events/%s/ack", strings.TrimRight(c.config.BaseURL, "/"), req.EventID)
	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := r.SetBody(req).Post(endpoint)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
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
	return c.GetPrivateDataContext(context.TODO(), server, dataType, userID, platform, platformUserID)
}

// GetPrivateDataContext is GetPrivateData with request cancellation and tracing.
func (c *HarukiToolboxClient) GetPrivateDataContext(ctx context.Context, server string, dataType ToolboxDataType, userID int64, platform, platformUserID string) ([]byte, error) {
	data, _, err := c.getPrivateData(ctx, server, dataType, userID, platform, platformUserID, 0)
	return data, err
}

// GetPrivateDataConditionalContext fetches a private-data snapshot unless the
// caller's known upload_time still matches upstream, in which case it returns
// (nil, true, nil) without transferring the payload.
//
// When toolbox.conditional_fetch is enabled the check rides the data request
// itself via ?known_upload_time (the Toolbox answers 304 Not Modified after
// running its full authorization). When disabled — for Toolbox deployments
// without conditional read support — the same contract is emulated with the
// legacy two-hop flow: an authorized key=upload_time probe followed by a full
// fetch when the value differs. The two modes are equivalent except for the
// upstream same-second guard, which only conditional mode inherits: a snapshot
// replaced within the same second as its upload_time is re-sent (200) there,
// while the emulation — like the legacy flow it preserves — sees an equal
// timestamp and reports notModified. A knownUploadTime of 0 always performs a
// plain full fetch.
func (c *HarukiToolboxClient) GetPrivateDataConditionalContext(ctx context.Context, server string, dataType ToolboxDataType, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error) {
	if knownUploadTime > 0 && c != nil && c.config != nil && c.config.ConditionalFetch {
		data, notModified, err := c.getPrivateData(ctx, server, dataType, userID, platform, platformUserID, knownUploadTime)
		if err == nil {
			if notModified {
				commandtrace.RecordOperation(ctx, "toolbox.conditional_not_modified", 0)
			} else {
				commandtrace.RecordOperation(ctx, "toolbox.conditional_changed", 0)
			}
		}
		return data, notModified, err
	}
	if knownUploadTime > 0 {
		raw, err := c.GetUploadTimeContext(ctx, server, dataType, userID, platform, platformUserID)
		if err != nil {
			return nil, false, err
		}
		current, err := parseUploadTimeBytes(raw)
		if err != nil {
			return nil, false, err
		}
		if current == knownUploadTime {
			return nil, true, nil
		}
	}
	data, _, err := c.getPrivateData(ctx, server, dataType, userID, platform, platformUserID, 0)
	return data, false, err
}

// GetSuiteDataConditionalContext is the conditional form of GetSuiteDataContext.
func (c *HarukiToolboxClient) GetSuiteDataConditionalContext(ctx context.Context, server string, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error) {
	return c.GetPrivateDataConditionalContext(ctx, server, ToolboxDataTypeSuite, userID, platform, platformUserID, knownUploadTime)
}

// GetMySekaiDataConditionalContext is the conditional form of GetMySekaiDataContext.
func (c *HarukiToolboxClient) GetMySekaiDataConditionalContext(ctx context.Context, server string, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error) {
	return c.GetPrivateDataConditionalContext(ctx, server, ToolboxDataTypeMySekai, userID, platform, platformUserID, knownUploadTime)
}

// getPrivateData performs one private game-data read. A positive
// knownUploadTime is forwarded as the known_upload_time query parameter, in
// which case a 304 response reports notModified=true with no payload.
func (c *HarukiToolboxClient) getPrivateData(ctx context.Context, server string, dataType ToolboxDataType, userID int64, platform, platformUserID string, knownUploadTime int64) ([]byte, bool, error) {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return nil, false, err
	}
	url := fmt.Sprintf("%s/api/private/game-data/%s/%s/%d", c.config.BaseURL, server, string(dataType), userID)
	params := map[string]string{
		"platform":         platform,
		"platform_user_id": platformUserID,
	}
	if knownUploadTime > 0 {
		params["known_upload_time"] = strconv.FormatInt(knownUploadTime, 10)
	}

	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := r.
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(params).
		Get(url)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("toolbox: request failed after retries: %w", sanitizeNetworkError(err))
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		data, err := decompressContext(ctx, resp)
		return data, false, err
	case http.StatusNotModified:
		if knownUploadTime <= 0 {
			return nil, false, &ToolboxAPIError{StatusCode: http.StatusNotModified, Message: "unexpected 304 without known_upload_time"}
		}
		return nil, true, nil
	default:
		return nil, false, mapPrivateDataStatusError(ctx, resp)
	}
}

// mapPrivateDataStatusError converts a non-2xx private game-data response into
// the package's typed errors, mirroring the Toolbox API's error vocabulary.
func mapPrivateDataStatusError(ctx context.Context, resp *resty.Response) error {
	switch resp.StatusCode() {
	case http.StatusForbidden:
		msg := parseMessage(toolboxResponseBodyContext(ctx, resp))
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return ErrAccountOwnerBanned
		default:
			return &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}

	case http.StatusNotFound:
		msg := parseMessage(toolboxResponseBodyContext(ctx, resp))
		switch {
		case strings.Contains(msg, "account binding not found"):
			return ErrAccountBindingNotFound
		case strings.Contains(msg, "game data not found"):
			return ErrGameDataNotFound
		default:
			return &ToolboxAPIError{StatusCode: http.StatusNotFound, Message: msg}
		}

	case http.StatusServiceUnavailable:
		return &ToolboxAPIError{StatusCode: http.StatusServiceUnavailable, Message: parseToolboxErrorMessageContext(ctx, resp, "toolbox service unavailable")}

	default:
		return &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: parseMessage(toolboxResponseBodyContext(ctx, resp))}
	}
}

// GetSuiteData fetches the user game-data snapshot (suite) from the Toolbox.
// The returned JSON is the raw payload equivalent to user.json, fed into userdata.Service for rendering.
func (c *HarukiToolboxClient) GetSuiteData(server string, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateData(server, ToolboxDataTypeSuite, userID, platform, platformUserID)
}

// GetSuiteDataContext is GetSuiteData with request cancellation and tracing.
func (c *HarukiToolboxClient) GetSuiteDataContext(ctx context.Context, server string, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateDataContext(ctx, server, ToolboxDataTypeSuite, userID, platform, platformUserID)
}

// GetMySekaiData fetches the MySekai world snapshot from the Toolbox.
// The returned JSON is the raw payload equivalent to mysekai.json, fed into the mysekai render controller.
func (c *HarukiToolboxClient) GetMySekaiData(server string, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateData(server, ToolboxDataTypeMySekai, userID, platform, platformUserID)
}

// GetMySekaiDataContext is GetMySekaiData with request cancellation and tracing.
func (c *HarukiToolboxClient) GetMySekaiDataContext(ctx context.Context, server string, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateDataContext(ctx, server, ToolboxDataTypeMySekai, userID, platform, platformUserID)
}

// GetPrivateDataValue queries a single top-level key from a private data snapshot.
//
// The server returns the raw value (e.g. a number or string) rather than the full JSON payload.
// This is efficient for lightweight lookups such as reading upload_time without fetching the
// entire snapshot.
//
//	GET /api/private/{server}/{dataType}/{userID}?platform=...&platform_user_id=...&key={key}
func (c *HarukiToolboxClient) GetPrivateDataValue(server string, dataType ToolboxDataType, userID int64, platform, platformUserID, key string) ([]byte, error) {
	return c.GetPrivateDataValueContext(context.TODO(), server, dataType, userID, platform, platformUserID, key)
}

// GetPrivateDataValueContext is GetPrivateDataValue with request cancellation and tracing.
func (c *HarukiToolboxClient) GetPrivateDataValueContext(ctx context.Context, server string, dataType ToolboxDataType, userID int64, platform, platformUserID, key string) ([]byte, error) {
	r, err := c.internalRequest(ctx)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/private/game-data/%s/%s/%d", c.config.BaseURL, server, string(dataType), userID)

	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := r.
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
			"key":              key,
		}).
		Get(url)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("toolbox: request failed after retries: %w", sanitizeNetworkError(err))
	}

	if resp.StatusCode() == http.StatusOK {
		return decompressContext(ctx, resp)
	}
	return nil, mapPrivateDataStatusError(ctx, resp)
}

// GetPrivateDataValues queries multiple top-level keys from a private data snapshot.
//
// The server returns a JSON object mapping each key to its value.
// Multiple keys are joined with commas: e.g. key=upload_time,version
func (c *HarukiToolboxClient) GetPrivateDataValues(server string, dataType ToolboxDataType, userID int64, platform, platformUserID string, keys ...string) ([]byte, error) {
	return c.GetPrivateDataValue(server, dataType, userID, platform, platformUserID, strings.Join(keys, ","))
}

// GetPrivateDataValuesContext is GetPrivateDataValues with request cancellation and tracing.
func (c *HarukiToolboxClient) GetPrivateDataValuesContext(ctx context.Context, server string, dataType ToolboxDataType, userID int64, platform, platformUserID string, keys ...string) ([]byte, error) {
	return c.GetPrivateDataValueContext(ctx, server, dataType, userID, platform, platformUserID, strings.Join(keys, ","))
}

// GetUploadTime fetches the upload_time timestamp (seconds) for a given data type snapshot.
// Returns the raw bytes of the integer value, e.g. []byte("1774339266").
func (c *HarukiToolboxClient) GetUploadTime(server string, dataType ToolboxDataType, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateDataValue(server, dataType, userID, platform, platformUserID, "upload_time")
}

// GetUploadTimeContext is GetUploadTime with request cancellation and tracing.
func (c *HarukiToolboxClient) GetUploadTimeContext(ctx context.Context, server string, dataType ToolboxDataType, userID int64, platform, platformUserID string) ([]byte, error) {
	return c.GetPrivateDataValueContext(ctx, server, dataType, userID, platform, platformUserID, "upload_time")
}

// parseUploadTimeBytes parses the raw upload_time value (a bare integer in
// seconds, e.g. []byte("1774339266")) returned by the key=upload_time query.
func parseUploadTimeBytes(raw []byte) (int64, error) {
	ts, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("toolbox: invalid upload_time value: %w", err)
	}
	return ts, nil
}

// GetSuiteUploadTimeContext returns the parsed suite upload_time (seconds).
// This is authorized identically to GetSuiteDataContext (same platform /
// platform_user_id gate), so a successful result confirms the caller is
// authorized for this account's suite data — it is a cheap freshness/authorization
// probe that avoids transferring the full snapshot.
func (c *HarukiToolboxClient) GetSuiteUploadTimeContext(ctx context.Context, server string, userID int64, platform, platformUserID string) (int64, error) {
	raw, err := c.GetUploadTimeContext(ctx, server, ToolboxDataTypeSuite, userID, platform, platformUserID)
	if err != nil {
		return 0, err
	}
	return parseUploadTimeBytes(raw)
}

// GetMySekaiUploadTimeContext returns the parsed mysekai upload_time (seconds).
// Authorized identically to GetMySekaiDataContext.
func (c *HarukiToolboxClient) GetMySekaiUploadTimeContext(ctx context.Context, server string, userID int64, platform, platformUserID string) (int64, error) {
	raw, err := c.GetUploadTimeContext(ctx, server, ToolboxDataTypeMySekai, userID, platform, platformUserID)
	if err != nil {
		return 0, err
	}
	return parseUploadTimeBytes(raw)
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
	return c.GetToolboxUserFastVerificationGameAccountBindingsContext(context.TODO(), platform, platformUserID)
}

// GetToolboxUserFastVerificationGameAccountBindingsContext is the cancellable,
// traced form of GetToolboxUserFastVerificationGameAccountBindings.
func (c *HarukiToolboxClient) GetToolboxUserFastVerificationGameAccountBindingsContext(ctx context.Context, platform, platformUserID string) ([]UserGameBinding, error) {
	request, err := c.internalRequest(ctx)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/private/game-binding", c.config.BaseURL)

	finishHTTP := commandtrace.MeasureOperation(ctx, "toolbox.http")
	resp, err := request.
		SetHeader("Accept-Encoding", "zstd").
		SetQueryParams(map[string]string{
			"platform":         platform,
			"platform_user_id": platformUserID,
		}).
		Get(url)
	finishHTTP()
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("toolbox: request failed after retries: %w", sanitizeNetworkError(err))
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		var bindings []UserGameBinding
		body, err := decompressContext(ctx, resp)
		if err != nil {
			return nil, err
		}
		finishDecode := commandtrace.MeasureOperation(ctx, "toolbox.decode")
		if err := sonic.Unmarshal(body, &bindings); err != nil {
			finishDecode()
			return nil, fmt.Errorf("toolbox: failed to parse game bindings response: %w", err)
		}
		finishDecode()
		return bindings, nil

	case http.StatusForbidden:
		msg := parseMessage(toolboxResponseBodyContext(ctx, resp))
		switch {
		case strings.Contains(msg, "invalid platform or platform_user_id"):
			return nil, ErrInvalidPlatformUser
		case strings.Contains(msg, "account owner is banned"):
			return nil, ErrAccountOwnerBanned
		default:
			return nil, &ToolboxAPIError{StatusCode: http.StatusForbidden, Message: msg}
		}

	case http.StatusNotFound:
		msg := parseMessage(toolboxResponseBodyContext(ctx, resp))
		if strings.Contains(msg, "account binding not found") {
			return nil, ErrAccountBindingNotFound
		}
		return nil, &ToolboxAPIError{StatusCode: http.StatusNotFound, Message: msg}

	case http.StatusServiceUnavailable:
		return nil, &ToolboxAPIError{StatusCode: http.StatusServiceUnavailable, Message: parseToolboxErrorMessageContext(ctx, resp, "toolbox service unavailable")}

	default:
		msg := parseMessage(toolboxResponseBodyContext(ctx, resp))
		return nil, &ToolboxAPIError{StatusCode: resp.StatusCode(), Message: msg}
	}
}

func toolboxResponseBodyContext(ctx context.Context, resp *resty.Response) []byte {
	body, err := decompressContext(ctx, resp)
	if err != nil {
		return resp.Body()
	}
	return body
}

func parseToolboxErrorMessageContext(ctx context.Context, resp *resty.Response, fallback string) string {
	msg := strings.TrimSpace(parseMessage(toolboxResponseBodyContext(ctx, resp)))
	if msg == "" {
		return fallback
	}
	return msg
}

// decompressContext handles transparent zstd decompression when the server indicates it.
func decompressContext(ctx context.Context, resp *resty.Response) ([]byte, error) {
	return decompressContextLimit(ctx, resp, toolboxMaxDecompressedResponseBytes)
}

func decompressContextLimit(ctx context.Context, resp *resty.Response, limit int64) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("toolbox: response is nil")
	}
	body := resp.Body()
	if resp.Header().Get("Content-Encoding") != "zstd" {
		if limit > 0 && int64(len(body)) > limit {
			return nil, fmt.Errorf("toolbox: response exceeds %d-byte limit", limit)
		}
		return body, nil
	}
	finishDecompress := commandtrace.MeasureOperation(ctx, "toolbox.decompress")
	defer finishDecompress()

	decoder, err := zstd.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd reader init failed: %w", err)
	}
	defer decoder.Close()

	reader := io.Reader(decoder)
	if limit > 0 {
		reader = io.LimitReader(decoder, limit+1)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("toolbox: zstd decompression failed: %w", err)
	}
	if limit > 0 && int64(len(out)) > limit {
		return nil, fmt.Errorf("toolbox: decompressed response exceeds %d-byte limit", limit)
	}
	return out, nil
}
