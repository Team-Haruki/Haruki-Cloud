package pjsk

import (
	"context"
	"strings"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/logger"
	"haruki-cloud/utils/usererror"
)

// commandErrorText is the reply for a failed command. Only typed user errors
// carry text for users; an upstream failure is classified into one, and any
// other error becomes the generic reply. Its cause only reaches logs.
//
// commandPath selects the route's guidance for usererror.Unrecognized and
// trigger (the command the user typed) the help pointer added to usage and
// bad-parameter errors.
func commandErrorText(ctx context.Context, err error, commandPath, trigger string) string {
	locale := i18n.LocaleFromContext(ctx)
	typed := userErrorOf(err)
	text := withRouteGuidance(typed, commandPath, trigger).In(locale)
	return sanitizeErrorReply(ctx, text, locale)
}

// userErrorOf returns err's typed user error: its own, the classification of
// an upstream failure, or the generic internal error.
func userErrorOf(err error) *usererror.Error {
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	return usererror.Internal(err)
}

// withRouteGuidance completes a usage or bad-parameter error: a generic
// "unrecognized arguments" reason becomes the route's own guidance, and the
// help pointer for the typed command follows the reason.
func withRouteGuidance(err *usererror.Error, commandPath, trigger string) i18n.Message {
	message := err.Message
	if err.Code != usererror.CodeUsage && err.Code != usererror.CodeBadParam {
		return message
	}
	if message.ID == "common.with_usage" {
		return message
	}
	if message.ID == "common.unrecognized_args" {
		if guidance, ok := guidanceForRoute(commandPath); ok {
			message = guidance
		}
	}
	trigger = helpTrigger(trigger)
	if trigger == "" {
		return message
	}
	return i18n.WithUsage(message, trigger)
}

// helpTrigger is the command as typed, without arguments, when it is a slash
// command.
func helpTrigger(trigger string) string {
	fields := strings.Fields(trigger)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return ""
	}
	return fields[0]
}

// sanitizeErrorReply is the last check before error text reaches a user: a
// line that no catalog message produces is dropped (and logged), and an
// empty result or a sensitive URL becomes the generic reply.
func sanitizeErrorReply(ctx context.Context, text string, locale i18n.Locale) string {
	clean, dropped := i18n.SanitizeLines(text)
	if len(dropped) > 0 {
		logger.WarnContext(ctx, "dropped non-catalog lines from an error reply",
			"dropped_lines", len(dropped),
			"first_dropped", usererror.RedactForLog(dropped[0], 160),
		)
	}
	if clean == "" || usererror.MessageContainsSensitiveURL(clean) {
		return i18n.RequestFailed().In(locale)
	}
	return clean
}
