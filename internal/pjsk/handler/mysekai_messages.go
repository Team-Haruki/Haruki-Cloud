package handler

// normalizeMySekaiUserFacingError is the reply for a failed MySekai command.
// The MySekai render layer returns typed errors for every case a user can act
// on; a missing snapshot becomes the "upload your data" reply, a renderer
// without the view (HTTP 404) the misconfiguration reply, and upstream
// failures are classified.
func normalizeMySekaiUserFacingError(err error, mode string) error {
	return WrapDomainError(err)
}
