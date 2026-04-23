package eventutil

// EffectiveClosedAt returns the event close timestamp used for "belongs to this event"
// checks. Older fixtures may omit closedAt, so we fall back to aggregateAt+1000ms.
func EffectiveClosedAt(aggregateAt, closedAt int64) int64 {
	if closedAt > 0 {
		return closedAt
	}
	if aggregateAt > 0 {
		return aggregateAt + 1000
	}
	return 0
}

func IsCurrent(startAt, aggregateAt, closedAt, now int64) bool {
	endAt := EffectiveClosedAt(aggregateAt, closedAt)
	return startAt > 0 && endAt > 0 && startAt <= now && now <= endAt
}

func IsRankingOpen(startAt, aggregateAt, now int64) bool {
	return startAt > 0 && aggregateAt > 0 && startAt <= now && now <= aggregateAt
}

func IsPast(aggregateAt, closedAt, now int64) bool {
	endAt := EffectiveClosedAt(aggregateAt, closedAt)
	return endAt > 0 && endAt < now
}
