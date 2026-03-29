package sekai

import (
	"encoding/json"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func makeResolvedCmd(ctx SekaiHandlerContext, module parser.TargetModule, mode string) *parser.ResolvedCommand {
	return &parser.ResolvedCommand{
		Module:            module,
		Mode:              mode,
		Query:             ctx.GetArgs(),
		Region:            string(ctx.Region()),
		RegionExplicit:    ctx.HasExplicitRegion(),
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

// embedSelfQuery resolves self-only query params from ctx and merges them
// into the given params map so the backend can resolve the correct binding.
func embedSelfQuery(params map[string]any, ctx SekaiHandlerContext) error {
	p, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return err
	}
	params["mode"] = p.Mode
	params["platform"] = p.Platform
	params["platform_user_id"] = p.PlatformUserID
	if p.Selector != "" {
		params["selector"] = p.Selector
	}
	return nil
}

// AllRegions returns all supported region values.
var AllRegions = []renderregion.Value{
	renderregion.JP,
	renderregion.CN,
	renderregion.TW,
	renderregion.KR,
	renderregion.EN,
}
