package releasecheck

import (
	"fmt"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

const (
	KindCard  = "card"
	KindMusic = "music"
	KindEvent = "event"
	KindGacha = "gacha"
)

type UnreleasedError struct {
	Kind  string
	Query string
	ID    int
}

func (e *UnreleasedError) Error() string {
	if e == nil {
		return "unreleased content"
	}
	switch {
	case e.ID > 0:
		return fmt.Sprintf("%s unreleased: %d", e.Kind, e.ID)
	case e.Query != "":
		return fmt.Sprintf("%s unreleased: %s", e.Kind, e.Query)
	default:
		return fmt.Sprintf("%s unreleased", e.Kind)
	}
}

// Unwrap exposes the typed user reply for the kind of content, so the
// reply layer shows "not released yet" without matching any text.
func (e *UnreleasedError) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Kind {
	case KindCard:
		return usererror.New(usererror.CodeNotFound, i18n.M("card.unreleased"))
	case KindMusic:
		return usererror.New(usererror.CodeNotFound, i18n.M("music.unreleased"))
	case KindEvent:
		return usererror.New(usererror.CodeNotFound, i18n.M("event.unreleased"))
	case KindGacha:
		return usererror.New(usererror.CodeNotFound, i18n.M("gacha.unreleased"))
	default:
		return nil
	}
}

func New(kind, query string, id int) error {
	return &UnreleasedError{
		Kind:  kind,
		Query: query,
		ID:    id,
	}
}
