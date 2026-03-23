package pjsk

// CommandRequest is the request body for POST /internal/pjsk/command (render commands only).
// Bot sends a raw text command; Cloud parses, resolves, renders, and returns the image.
// Account management commands (bind/unbind/etc.) use dedicated REST endpoints, not this one.
type CommandRequest struct {
	IMPlatform string `json:"im_platform"`
	IMUserID   string `json:"im_user_id"`
	Command    string `json:"command"`
	Server     string `json:"server,omitempty"`
}

// CommandErrorResponse is returned when the command cannot be processed.
type CommandErrorResponse struct {
	Error string `json:"error"`
	Mode  string `json:"mode,omitempty"`
}
