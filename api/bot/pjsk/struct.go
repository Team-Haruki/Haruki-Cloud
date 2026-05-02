package pjsk

import "haruki-cloud/internal/onebot11"

// BotCommandRequest is the unified POST body for per-feature bot endpoints.
// All fields that were previously split across query params and request headers
// are now carried in a single JSON (transitioning to MsgPack) body.
type BotCommandRequest struct {
	Platform        string           `json:"platform" msgpack:"platform"`
	PlatformUserID  string           `json:"platform_user_id" msgpack:"platform_user_id"`
	PlatformGroupID string           `json:"platform_group_id,omitempty" msgpack:"platform_group_id,omitempty"`
	Server          string           `json:"server,omitempty" msgpack:"server,omitempty"`
	MatchedCommand  string           `json:"matched_command" msgpack:"matched_command"`
	Message         onebot11.Message `json:"message" msgpack:"message"`
	// EnableParamEcho allows clients to opt in to receiving the concrete
	// parameter text in parse-error responses. The default is false.
	EnableParamEcho bool `json:"enableParamEcho,omitempty" msgpack:"enableParamEcho,omitempty"`
}

// BotCommandErrorResponse is returned when a bot endpoint cannot process the request.
type BotCommandErrorResponse struct {
	Error          string `json:"error" msgpack:"error"`
	Mode           string `json:"mode,omitempty" msgpack:"mode,omitempty"`
	ExpectedModule string `json:"expected_module,omitempty" msgpack:"expected_module,omitempty"`
	ExpectedMode   string `json:"expected_mode,omitempty" msgpack:"expected_mode,omitempty"`
	ExpectedPath   string `json:"expected_path,omitempty" msgpack:"expected_path,omitempty"`
	MatchedCommand string `json:"matched_command,omitempty" msgpack:"matched_command,omitempty"`
}

// ManifestEntry describes one feature endpoint served by the Bot API.
// The Bot client downloads the full manifest at startup and uses it to
// pre-match user commands to the correct endpoint path without a server round-trip.
type ManifestEntry struct {
	// CommandPrefixes is the list of text prefixes (or patterns) that trigger this endpoint,
	// e.g. ["/查卡", "/card", "/cards"].
	CommandPrefixes []string `json:"command_prefixes" msgpack:"command_prefixes"`

	// CommandPriority controls matching order; higher value is matched first.
	CommandPriority int `json:"command_priority" msgpack:"command_priority"`

	// CommandMode is the accepted HTTP method, e.g. "GET".
	CommandMode string `json:"command_mode" msgpack:"command_mode"`

	// CommandModule is the top-level game module, e.g. "pjsk", "chunithm".
	CommandModule string `json:"command_module" msgpack:"command_module"`

	// CommandPath is the path relative to the module base (no leading slash),
	// e.g. "card/detail". Full URL: /api/v2/bot/{botId}/{module}/{path}.
	CommandPath string `json:"command_path" msgpack:"command_path"`

	// CommandAdditionalParams is an optional list of extra query parameter names
	// the endpoint accepts beyond the standard command payload protocol.
	CommandAdditionalParams []string `json:"command_additional_params,omitempty" msgpack:"command_additional_params,omitempty"`
}

// ManifestResponse is returned by GET /api/v2/bot/:botId/command/manifests.
type ManifestResponse struct {
	Entries                   []ManifestEntry `json:"entries" msgpack:"entries"`
	CurrentHarukiCloudVersion string          `json:"currentHarukiCloudVersion" msgpack:"currentHarukiCloudVersion"`
	LatestHarukiClientVersion string          `json:"latestHarukiClientVersion" msgpack:"latestHarukiClientVersion"`
	Profile                   string          `json:"profile" msgpack:"profile"`
}
