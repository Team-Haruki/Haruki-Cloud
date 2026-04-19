package handler

type deckEventLockedError struct {
	EventID int
}

func (e *deckEventLockedError) Error() string {
	if e == nil || e.EventID <= 0 {
		return "deck event is locked until gacha release"
	}
	return "deck event is locked until gacha release"
}
