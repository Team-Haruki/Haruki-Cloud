package pjsk

// BotCommandRequest is the canonical request shape for per-feature bot endpoints.
// The client sends the raw OneBot v11 payload as a Base64-encoded query parameter
// and carries routing / platform metadata in request headers.
type BotCommandRequest struct {
	Platform        string
	PlatformUserID  string
	PlatformGroupID string
	CommandPayload  string
	MatchedCommand  string
	Server          string
}

// BotCommandErrorResponse is returned when a bot endpoint cannot process the request.
type BotCommandErrorResponse struct {
	Error          string `json:"error"`
	Mode           string `json:"mode,omitempty"`
	ExpectedModule string `json:"expected_module,omitempty"`
	ExpectedMode   string `json:"expected_mode,omitempty"`
	ExpectedPath   string `json:"expected_path,omitempty"`
	MatchedCommand string `json:"matched_command,omitempty"`
}

// ManifestEntry describes one feature endpoint served by the Bot API.
// The Bot client downloads the full manifest at startup and uses it to
// pre-match user commands to the correct endpoint path without a server round-trip.
type ManifestEntry struct {
	// CommandPrefixes is the list of text prefixes (or patterns) that trigger this endpoint,
	// e.g. ["/查卡", "/card", "/cards"].
	CommandPrefixes []string `json:"command_prefixes"`

	// CommandPriority controls matching order; higher value is matched first.
	CommandPriority int `json:"command_priority"`

	// CommandMode is the accepted HTTP method, e.g. "GET".
	CommandMode string `json:"command_mode"`

	// CommandModule is the top-level game module, e.g. "pjsk", "chunithm".
	CommandModule string `json:"command_module"`

	// CommandPath is the path relative to the module base (no leading slash),
	// e.g. "card/detail". Full URL: /api/v2/bot/{botId}/{module}/{path}.
	CommandPath string `json:"command_path"`

	// CommandAdditionalParams is an optional list of extra query parameter names
	// the endpoint accepts beyond the standard command payload protocol.
	CommandAdditionalParams []string `json:"command_additional_params,omitempty"`
}

// ManifestResponse is returned by GET /api/v2/bot/:botId/command/manifests.
type ManifestResponse struct {
	Entries []ManifestEntry `json:"entries"`
}
