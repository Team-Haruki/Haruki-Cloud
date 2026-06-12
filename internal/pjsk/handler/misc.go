package handler

import (
	"fmt"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/requestbuilder"
	"strconv"
	"strings"
)

type miscBirthdayParams struct {
	Cid           int    `json:"cid,omitempty"`
	UpcomingIndex int    `json:"upcoming_index,omitempty"`
	Query         string `json:"query,omitempty"`
}

func (sekaiHandlers) MiscBirthdayHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "misc/birthday",
			Commands: []string{
				"/pjsk chara birthday", "/角色生日", "/生日", "/查生日",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildMiscBirthdayParams(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMisc, "misc-birthday", params), nil
		},
	}, executeMisc)
}

func buildMiscBirthdayParams(args string) (miscBirthdayParams, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return miscBirthdayParams{UpcomingIndex: 1}, nil
	}

	if index, err := strconv.Atoi(args); err == nil {
		if index <= 0 || index > 26 {
			return miscBirthdayParams{}, fmt.Errorf("角色生日索引超出范围")
		}
		return miscBirthdayParams{UpcomingIndex: index}, nil
	}
	return miscBirthdayParams{Query: args}, nil
}

func (sekaiHandlers) ProfileHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "profile",
			Commands: []string{
				"/个人中心", "/profile", "/个人信息",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			p, err := resolveUserQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			p.ProfileVertical, _ = extractProfileVerticalArg(ctx.GetArgs())
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeRender, p), nil
		},
	}, executeProfile)
}

func executeMisc(rc *RequestContext) (message onebot11.Message, err error) {
	miscCtrl := rc.App.Misc.WithContext(rc.Ctx)
	var data []byte
	switch rc.Cmd.Mode {
	case "misc-birthday":
		req, resolveErr := requestbuilder.BuildMiscBirthdayRequest(rc.Ctx, toRequestBuilderCommandInput(rc.Cmd), rc.App)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = miscCtrl.RenderCharaBirthday(*req)
	default:
		return nil, unsupportedModeError("misc", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}
