package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/accountdata"
	"strconv"
	"strings"
)

type miscBirthdayParams struct {
	Cid           int    `json:"cid,omitempty"`
	UpcomingIndex int    `json:"upcoming_index,omitempty"`
	Query         string `json:"query,omitempty"`
}

func (sekaiHandlers) MiscBirthdayHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "misc/birthday",
			Commands: []string{
				"/pjsk chara birthday", "/角色生日", "/生日", "/查生日",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			params, err := buildMiscBirthdayParams(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMisc, "misc-birthday", params), nil
		},
	}
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

func (sekaiHandlers) ProfileHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "profile",
			Commands: []string{
				"/个人中心", "/profile", "/个人信息",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			p, err := resolveUserQueryParams(ctx)
			if err != nil {
				return nil, err
			}
			p.ProfileVertical, _ = extractProfileVerticalArg(ctx.GetArgs())
			return makeResolvedCmdWithParams(ctx, parser.ModuleProfile, accountdata.ProfileModeRender, p), nil
		},
	}
}
