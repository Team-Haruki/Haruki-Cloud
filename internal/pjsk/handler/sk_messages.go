package handler

// normalizeSKUserFacingError is the reply for a failed ranking command:
// typed errors pass through, upstream failures are classified, and a bare
// network failure is attributed to the ranking service.
func normalizeSKUserFacingError(err error) error {
	return normalizeTrackerUserFacingError(err)
}
