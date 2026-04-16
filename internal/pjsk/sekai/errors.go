package sekai

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

// errorBody is the shape of all non-2xx JSON responses from the APIs.
type errorBody struct {
	Message string `json:"message"`
}

// parseMessage extracts the "message" field from a JSON error body.
// Returns the raw body string if parsing fails.
func parseMessage(body []byte) string {
	var eb errorBody
	if err := sonic.Unmarshal(body, &eb); err == nil && eb.Message != "" {
		return eb.Message
	}
	return strings.TrimSpace(string(body))
}

// Sentinel errors for known Sekai API failure cases.
var (
	// ErrUserNotFound means the queried game user ID does not exist (HTTP 404).
	ErrUserNotFound = errors.New("sekai api: user not found")

	// ErrServerMaintenance means the game server is currently under maintenance (HTTP 503).
	ErrServerMaintenance = errors.New("sekai api: game server is under maintenance")

	// ErrClientNotConfigured is returned by any Sekai-family client when a method
	// is invoked on a nil receiver. Callers (especially tests) may construct an
	// App without wiring these clients; we return this error rather than panicking
	// on the nil receiver so existing error-handling paths kick in.
	ErrClientNotConfigured = errors.New("sekai client: not configured")
)

// SekaiAPIError is returned for unexpected non-2xx responses that do not map
// to one of the typed sentinel errors above.
type SekaiAPIError struct {
	StatusCode int
	Message    string
}

func (e *SekaiAPIError) Error() string {
	return fmt.Sprintf("sekai api error: status %d, message: %q", e.StatusCode, e.Message)
}

// Sentinel errors for known Tracker API failure cases.
var (
	// ErrRankingNotFound means the requested ranking record does not exist (HTTP 404).
	ErrRankingNotFound = errors.New("tracker: ranking record not found")
)

// TrackerAPIError is returned for non-2xx responses that do not map to a
// typed sentinel error.
type TrackerAPIError struct {
	StatusCode int
	Message    string
}

func (e *TrackerAPIError) Error() string {
	return fmt.Sprintf("tracker api error: status %d, message: %q", e.StatusCode, e.Message)
}

// Sentinel errors for known Toolbox API failure cases.
// Callers should use errors.Is() to check for specific conditions.
var (
	// ErrAccountBindingNotFound means the user has not bound a game account on the
	// upstream suite service (HTTP 404, message: "account binding not found").
	ErrAccountBindingNotFound = errors.New("account binding not found on suite service")

	// ErrGameDataNotFound means the user has not uploaded any game data yet
	// (HTTP 404, message: "game data not found").
	ErrGameDataNotFound = errors.New("game data not found: user has not uploaded data")

	// ErrInvalidPlatformUser means the requesting platform/user combination is not
	// authorised to access this game data (HTTP 403, message starts with
	// "forbidden: invalid platform or platform_user_id for this user").
	ErrInvalidPlatformUser = errors.New("forbidden: invalid platform or platform_user_id for this user")

	// ErrAccountOwnerBanned means the game account's owner has been banned
	// (HTTP 403, message: "forbidden: account owner is banned").
	ErrAccountOwnerBanned = errors.New("forbidden: account owner is banned")
)

// ToolboxAPIError is returned for unexpected non-2xx responses that do not map
// to one of the typed sentinel errors above.
type ToolboxAPIError struct {
	StatusCode int
	Message    string
}

func (e *ToolboxAPIError) Error() string {
	return fmt.Sprintf("toolbox api error: status %d, message: %q", e.StatusCode, e.Message)
}
