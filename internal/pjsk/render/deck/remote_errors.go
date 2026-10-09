package deck

import (
	"errors"
	"fmt"

	"haruki-cloud/internal/core/upstreamerr"
)

// RemoteError is an error answer of deck-service: a non-2xx HTTP response
// (StatusCode set) or an error item of a batch result (StatusCode 0).
// upstreamerr classifies it by status and message.
type RemoteError struct {
	StatusCode int
	Message    string
}

func (e *RemoteError) Error() string {
	switch {
	case e.StatusCode == 0:
		return e.Message
	case e.Message != "":
		return fmt.Sprintf("deck-service returned HTTP %d: %s", e.StatusCode, e.Message)
	default:
		return fmt.Sprintf("deck-service returned HTTP %d", e.StatusCode)
	}
}

func (e *RemoteError) UpstreamService() upstreamerr.Service { return upstreamerr.ServiceDeck }
func (e *RemoteError) UpstreamStatus() int                  { return e.StatusCode }
func (e *RemoteError) UpstreamMessage() string              { return e.Message }

// remoteJSONError is the "error" field of a deck-service error body: its
// Error() text is the message alone, as before.
type remoteJSONError struct{ RemoteError }

func (e *remoteJSONError) Error() string { return e.Message }

// ErrUserDataRequired is returned when a recommendation needs the user's
// game data (suite) and none was resolved; handlers turn it into the "upload
// your data" reply for the account.
var ErrUserDataRequired = errors.New("user data is required for deck auto recommend")

// errDeckNotConfigured is returned when no deck-service target is wired.
var errDeckNotConfigured error = upstreamerr.NewSentinel(upstreamerr.ServiceDeck, upstreamerr.KindNotConfigured, 0, "deck recommend service is not configured")

func deckTagged(kind upstreamerr.Kind, err error) error {
	return upstreamerr.Tag(upstreamerr.ServiceDeck, kind, "", err)
}
