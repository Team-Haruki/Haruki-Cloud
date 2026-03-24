package sekai

import (
	"encoding/json"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strings"
)

var currentNicknames map[string]int

func SetNicknames(nicknames map[string]int) {
	currentNicknames = nicknames
}

func makeResolvedCmd(ctx SekaiHandlerContext, module parser.TargetModule, mode string) *parser.ResolvedCommand {
	return &parser.ResolvedCommand{
		Module:            module,
		Mode:              mode,
		Query:             ctx.GetArgs(),
		Region:            string(ctx.Region()),
		IsHelp:            ctx.Flags()["is_help"],
		IsVerbose:         ctx.Flags()["is_verbose"],
		IsPreview:         ctx.Flags()["is_preview"],
		RequesterPlatform: ctx.Platform,
		RequesterUserID:   ctx.UserId,
	}
}

func makeResolvedCmdWithParams(ctx SekaiHandlerContext, module parser.TargetModule, mode string, params any) *parser.ResolvedCommand {
	resolved := makeResolvedCmd(ctx, module, mode)
	if params == nil {
		return resolved
	}
	if data, err := json.Marshal(params); err == nil {
		resolved.Params = data
	}
	return resolved
}

func resolveNicknameArg(args string) (int, string) {
	if len(currentNicknames) == 0 {
		return 0, strings.TrimSpace(args)
	}
	ext := parser.NewExtractor(currentNicknames)
	res := ext.ExtractCharacter(args)
	if res.Found {
		return res.Value, strings.TrimSpace(res.Remaining)
	}
	return 0, strings.TrimSpace(args)
}

// AllRegions returns all supported region values.
var AllRegions = []renderregion.Value{
	renderregion.JP,
	renderregion.CN,
	renderregion.TW,
	renderregion.KR,
	renderregion.EN,
}
