package releasecheck

import "fmt"

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

func New(kind, query string, id int) error {
	return &UnreleasedError{
		Kind:  kind,
		Query: query,
		ID:    id,
	}
}
