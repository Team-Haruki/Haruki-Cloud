package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/utils/usererror"

	json "haruki-cloud/internal/jsonutil"
)

const modeGlobalBack = "global-back"

type globalBackParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	QQID           string `json:"qq_id"`
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
