package snapshot

import "errors"

// ErrNotConfigured is returned when a command needs the user's game data
// (suite) but no snapshot was resolved for the request. Command handlers
// turn it into the "upload your data" reply for the right account.
var ErrNotConfigured = errors.New("local user snapshot is not configured")

// ErrMySekaiUnavailable is returned when a command needs the user's MySekai
// data but none was resolved for the request.
var ErrMySekaiUnavailable = errors.New("user snapshot is not available (bind Toolbox or provide snapshot)")
