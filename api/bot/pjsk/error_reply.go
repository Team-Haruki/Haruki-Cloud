package pjsk

import (
	"context"
	"errors"
	"strings"

	"haruki-cloud/internal/core/upstreamerr"
	commandregistry "haruki-cloud/internal/handler"
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
//
// Unless the client enabled parameter echo (i18n.WithParamEcho), the reply
// shows no user input: every message with a user-input placeholder is
// rendered in its echo-free form, and the help pointer names a registered
// command only.
func commandErrorText(ctx context.Context, err error, commandPath, trigger string) string {
	return commandErrorReply(ctx, err, commandPath, helpTrigger(trigger, i18n.ParamEchoFromContext(ctx)))
}

// commandErrorReply is commandErrorText with the help pointer's command
// already known (empty for none), for routes served outside the command
// registry.
func commandErrorReply(ctx context.Context, err error, commandPath, helpCommand string) string {
	typed := userErrorOf(err)
	return sanitizeErrorReply(ctx, withRouteGuidance(typed, commandPath, helpCommand), i18n.RenderOptionsFromContext(ctx))
}

// userErrorOf returns err's typed user error: its own, the classification of
// an upstream failure, a generic timeout for a request deadline, or the
// generic internal error.
func userErrorOf(err error) *usererror.Error {
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return usererror.Wrap(usererror.CodeTimeout, i18n.M("common.request_timeout"), err)
	}
	return usererror.Internal(err)
}

// withRouteGuidance completes a usage or bad-parameter error: a generic
// "unrecognized arguments" reason becomes the route's own guidance, and the
// help pointer for helpCommand (the command as typed, see helpTrigger)
// follows the reason.
func withRouteGuidance(err *usererror.Error, commandPath, helpCommand string) i18n.Message {
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
	if helpCommand == "" {
		return message
	}
	return i18n.WithUsage(message, helpCommand)
}

// helpTrigger is the command as typed, without arguments, when it is a slash
// command. A registered command is kept whole, so a multi-word command such
// as "/pjsk vlive" is not cut down to "/pjsk". An unregistered first word is
// user input, so it is used only when echo is enabled.
func helpTrigger(trigger string, echo bool) string {
	trigger = strings.Join(strings.Fields(trigger), " ")
	if !strings.HasPrefix(trigger, "/") {
		return ""
	}
	if matched := commandregistry.MatchCommandHandler(trigger); matched.Handler != nil {
		if command := strings.TrimSpace(matched.Command); command != "" {
			return command
		}
	}
	if !echo {
		return ""
	}
	return strings.Fields(trigger)[0]
}

// sanitizeErrorReply renders reply and is the last check before error text
// reaches a user: a line that no catalog message produces is dropped (and
// logged), and an empty result or a sensitive URL becomes the generic reply.
func sanitizeErrorReply(ctx context.Context, reply i18n.Message, opts i18n.RenderOptions) string {
	clean, dropped := i18n.SanitizeMessage(reply, opts)
	if len(dropped) > 0 {
		logger.WarnContext(ctx, "dropped non-catalog lines from an error reply",
			"dropped_lines", len(dropped),
			"first_dropped", usererror.RedactForLog(dropped[0], 160),
		)
	}
	if clean == "" || usererror.MessageContainsSensitiveURL(clean) {
		return i18n.RequestFailed().Render(opts)
	}
	return clean
}
