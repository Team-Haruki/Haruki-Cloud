package card

import (
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func currentCardVisibilityTime() int64 {
	return time.Now().UnixMilli()
}

func isCardVisibleAt(cardInfo *masterdata.Card, now int64) bool {
	if cardInfo == nil {
		return false
	}
	return cardInfo.ReleaseAt <= now
}

func filterVisibleCards(items []*masterdata.Card, now int64) []*masterdata.Card {
	if len(items) == 0 {
		return nil
	}
	filtered := make([]*masterdata.Card, 0, len(items))
	for _, item := range items {
		if !isCardVisibleAt(item, now) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func sortCardsByReleaseAndID(items []*masterdata.Card) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ReleaseAt == items[j].ReleaseAt {
			return items[i].ID < items[j].ID
		}
		return items[i].ReleaseAt < items[j].ReleaseAt
	})
}
