package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/utils/usererror"

	json "haruki-cloud/internal/jsonutil"
)

const (
	modeGlobalKill = "global-kill"
	modeGlobalBack = "global-back"
)

type globalKillParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	QQID           string `json:"qq_id"`
	Reason         string `json:"reason"`
	Days           *int   `json:"days,omitempty"`
}

type globalBackParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	QQID           string `json:"qq_id"`
}

func (sekaiHandlers) GlobalKillHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "admin/kill",
		Commands:    []string{"/kill"},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			qqID, reason, days, err := parseGlobalKillArgs(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			if ctx.GetPlatform() == "qq" && strings.TrimSpace(ctx.GetUserId()) == qqID {
				return nil, usererror.Forbidden(i18n.M("moderation.kill.self"))
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAdmin, modeGlobalKill, globalKillParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				QQID:           qqID,
				Reason:         reason,
				Days:           days,
			}), nil
		},
	}, executeGlobalModeration)
}

func (sekaiHandlers) GlobalBackHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "admin/back",
		Commands:    []string{"/back"},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			qqID, err := parseQQIDArg(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAdmin, modeGlobalBack, globalBackParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				QQID:           qqID,
			}), nil
		},
	}, executeGlobalModeration)
}

func parseGlobalKillArgs(args string) (string, string, *int, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) < 2 || len(fields) > 3 {
		return "", "", nil, usererror.Usage(i18n.M("moderation.kill.usage_reason"), "/kill")
	}
	qqID, err := validateQQID(fields[0])
	if err != nil {
		return "", "", nil, err
	}

	var days *int
	if len(fields) == 3 {
		parsed, err := strconv.Atoi(fields[2])
		if err != nil || parsed <= 0 {
			return "", "", nil, usererror.BadParam(fields[2], i18n.M("moderation.kill.days_invalid"))
		}
		days = &parsed
	}
	reason := strings.TrimSpace(fields[1])
	if reason == "" {
		return "", "", nil, usererror.Invalid(i18n.M("moderation.kill.reason_required"))
	}
	if len([]rune(reason)) > accountdata.BanReasonMaxRunes {
		return "", "", nil, usererror.Invalid(i18n.M("moderation.kill.reason_too_long", i18n.Data{"Max": accountdata.BanReasonMaxRunes}))
	}
	return qqID, reason, days, nil
}

func parseQQIDArg(args string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return "", usererror.Usage(i18n.M("moderation.back.usage_reason"), "/back")
	}
	return validateQQID(fields[0])
}

func validateQQID(value string) (string, error) {
	value = strings.TrimSpace(value)
	qqID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || qqID <= 0 {
		return "", usererror.BadParam(value, i18n.M("moderation.qq_invalid"))
	}
	return strconv.FormatInt(qqID, 10), nil
}

func executeGlobalModeration(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.App.BanChecker == nil {
		return nil, usererror.Misconfigured(usererror.Misconfigured(errors.New("global moderation: ban checker is not configured")))
	}
	switch rc.Cmd.Mode {
	case modeGlobalKill:
		var params globalKillParams
		if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
			return nil, usererror.Internal(fmt.Errorf("decode kill params: %w", err))
		}
		if !rc.App.BanChecker.IsAdmin(params.Platform, params.PlatformUserID) {
			return nil, usererror.Forbidden(i18n.M("moderation.not_admin"))
		}
		var expiresAt *time.Time
		if params.Days != nil {
			value := time.Now().AddDate(0, 0, *params.Days)
			expiresAt = &value
		}
		status, err := rc.App.BanChecker.Kill(rc.Ctx, params.QQID, params.Reason, expiresAt)
		if err != nil {
			return nil, moderationFailure(err)
		}
		message := i18n.M("moderation.kill.done_permanent", i18n.Data{"QQ": params.QQID, "Reason": status.Reason})
		if status.ExpiresAt != nil {
			message = i18n.M("moderation.kill.done_until", i18n.Data{
				"QQ":        params.QQID,
				"Reason":    status.Reason,
				"ExpiresAt": i18n.FormatUserTime(*status.ExpiresAt, displaytime.RequestLocation(rc.Ctx)),
			})
		}
		return onebot11.Message{onebot11.Text(message.String())}, nil
	case modeGlobalBack:
		var params globalBackParams
		if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
			return nil, usererror.Internal(fmt.Errorf("decode back params: %w", err))
		}
		if !rc.App.BanChecker.IsAdmin(params.Platform, params.PlatformUserID) {
			return nil, usererror.Forbidden(i18n.M("moderation.not_admin"))
		}
		if err := rc.App.BanChecker.Back(rc.Ctx, params.QQID); err != nil {
			return nil, moderationFailure(err)
		}
		return onebot11.Message{onebot11.Text(i18n.T("moderation.back.done", i18n.Data{"QQ": params.QQID}))}, nil
	default:
		return nil, usererror.Internal(fmt.Errorf("unsupported global moderation mode %q", rc.Cmd.Mode))
	}
}

// moderationFailure keeps typed errors from the ban service and hides any
// other error (database failures) behind the generic reply.
func moderationFailure(err error) error {
	if _, ok := usererror.As(err); ok {
		return err
	}
	return usererror.Internal(err)
}
