package pjsk

// BotCommandRequest is the common request body for all per-feature bot endpoints.
// GET requests may also pass these as query parameters.
// Command is expected to be a Base64-encoded OneBot JSON payload;
// if decoding fails, the raw string is treated as a plain text command.
type BotCommandRequest struct {
	IMPlatform string `json:"im_platform" query:"im_platform"`
	IMUserID   string `json:"im_user_id"  query:"im_user_id"`
	Command    string `json:"command"     query:"command"`
	Server     string `json:"server"      query:"server"`
}

// BotCommandErrorResponse is returned when a bot endpoint cannot process the request.
type BotCommandErrorResponse struct {
	Error          string `json:"error"`
	Mode           string `json:"mode,omitempty"`
	ExpectedModule string `json:"expected_module,omitempty"`
	ExpectedMode   string `json:"expected_mode,omitempty"`
}

// ManifestEntry describes one feature endpoint and the command prefixes that map to it.
// TODO: Full manifest structure TBD when command manifests are implemented.
type ManifestEntry struct {
	Endpoint string   `json:"endpoint"` // e.g. /api/v2/bot/{botId}/pjsk/card/detail
	Methods  []string `json:"methods"`  // e.g. ["GET","POST"]
	Module   string   `json:"module"`
	Mode     string   `json:"mode"`
	Prefixes []string `json:"prefixes"` // human-readable command prefixes
}

// ManifestResponse is returned by GET /api/v2/bot/:botId/command/manifests.
// TODO: Placeholder — full command manifest design pending.
type ManifestResponse struct {
	Entries []ManifestEntry `json:"entries"`
}
