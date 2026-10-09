package i18n

import "strings"

// Feature names used by the generic Unavailable and Timeout messages. They
// name what the user asked for, never an internal component.
var (
	FeatureGameData = M("common.feature.game_data")
	FeatureRanking  = M("common.feature.ranking")
	FeatureRender   = M("common.feature.render")
	FeatureToolbox  = M("common.feature.toolbox")
	FeatureDeck     = M("common.feature.deck")
	FeatureAccount  = M("common.feature.account")
)

// RequestFailed is the generic reply for a failure the user cannot act on.
func RequestFailed() Message { return M("common.request_failed") }

// Misconfigured is the reply for a problem only the bot owner can fix; the
// detail belongs in logs and commandtrace.
func Misconfigured() Message { return M("common.misconfigured") }

// ReadOnly is the reply when this service currently refuses data changes.
func ReadOnly() Message { return M("common.read_only") }

// Unavailable reports that a feature cannot be used right now.
func Unavailable(feature Message) Message {
	return M("common.unavailable", Data{"Feature": feature})
}

// Timeout reports that a feature did not answer in time.
func Timeout(feature Message) Message {
	return M("common.timeout", Data{"Feature": feature})
}

// NotFound reports that nothing of kind matches the user's query.
func NotFound(kind Message, query string) Message {
	return M("common.not_found", Data{"Kind": kind, "Query": EchoQuery(query)})
}

// Ambiguous reports that the user's query matches several things of kind.
func Ambiguous(kind Message, query string) Message {
	return M("common.ambiguous", Data{"Kind": kind, "Query": EchoQuery(query)})
}

// OutOfRange reports that a numeric parameter lies outside min~max.
func OutOfRange(param Message, minValue, maxValue int) Message {
	return M("common.out_of_range", Data{"Param": param, "Min": minValue, "Max": maxValue})
}

// BadParam is the reply for a malformed parameter: a first line naming the
// parameter as the user wrote it, then the specific reason.
func BadParam(param string, reason Message) Message {
	return M("common.bad_param", Data{"Param": EchoQuery(param), "Reason": reason})
}

// Usage is the pointer to a command's help, e.g. "发送 /查曲 -help 查看用法".
// trigger is the command as users type it, with or without the leading "/".
func Usage(trigger string) Message {
	trigger = strings.TrimSpace(trigger)
	if trigger != "" && !strings.HasPrefix(trigger, "/") {
		trigger = "/" + trigger
	}
	return M("common.usage", Data{"Trigger": trigger})
}

// WithUsage appends the help pointer for trigger to a one-line reason.
func WithUsage(reason Message, trigger string) Message {
	return M("common.with_usage", Data{"Reason": reason, "Usage": Usage(trigger)})
}
