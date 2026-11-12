package handler

import "strings"

func extractDeckMemoryEvent(args string, params *deckAutoQueryParams, mode string) string {
	if mode != deckEventCommand && mode != deckBonusCommand && mode != deckMySekaiCommand {
		return args
	}
	fields := strings.Fields(args)
	// Only the complete leading selector is reserved; "me" inside a song query
	// (or on its own) must reach the normal music alias resolver unchanged.
	if len(fields) < 2 || !strings.EqualFold(fields[0], "me") {
		return args
	}
	unit, ok := educationAreaUnitAliases[strings.ToLower(fields[1])]
	if !ok {
		return args
	}
	params.EventMEUnit = unit
	return strings.Join(fields[2:], " ")
}
