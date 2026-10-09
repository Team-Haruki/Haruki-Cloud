package handler

import (
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

type deckEventLockedError struct {
	EventID int
}

func (e *deckEventLockedError) Error() string {
	return "deck event is locked until gacha release"
}

// Unwrap exposes the typed user reply.
func (e *deckEventLockedError) Unwrap() error {
	return usererror.Forbidden(i18n.M("deck.event.locked"))
}
