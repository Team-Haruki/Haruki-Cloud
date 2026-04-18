package handler

import (
	"encoding/json"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func makeCommandRequest(ctx HarrukiSekaiHandlerContext, module parser.TargetModule, mode string) *CommandRequest {
	return &CommandRequest{
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

func makeCommandRequestWithParams(ctx HarrukiSekaiHandlerContext, module parser.TargetModule, mode string, params any) *CommandRequest {
	resolved := makeCommandRequest(ctx, module, mode)
	if params == nil {
		return resolved
	}
	if data, err := json.Marshal(params); err == nil {
		resolved.Params = data
	}
	return resolved
}

func newSelfQueryParamsMap(ctx HarrukiSekaiHandlerContext) (map[string]any, error) {
	params := map[string]any{}
	if err := embedSelfQuery(params, ctx); err != nil {
		return nil, err
	}
	return params, nil
}

// embedSelfQuery resolves self-only query params from ctx and merges them
// into the given params map so the backend can resolve the correct binding.
func embedSelfQuery(params map[string]any, ctx HarrukiSekaiHandlerContext) error {
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

// AllRegions lists every supported region; used as the default region set
// when a HarukiSekaiCommandHandler leaves Regions empty.
var AllRegions = []renderregion.Value{
	renderregion.JP,
	renderregion.CN,
	renderregion.TW,
	renderregion.KR,
	renderregion.EN,
}
