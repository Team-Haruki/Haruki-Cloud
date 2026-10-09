package handler

import (
	"errors"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/usererror"
)

func (sekaiHandlers) ChartHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "music/chart",
		Commands: []string{
			"/pjsk chart",
			"/谱面查询", "/铺面查询", "/谱面预览", "/铺面预览", "/谱面", "/铺面", "/查谱面", "/查铺面", "/查谱",
			"/技能预览",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) == "" {
				return nil, usererror.Misuse(i18n.M("music.query_required"))
			}
			if ctx.GetTriggerCmd() == "/技能预览" { //copylint:ignore 指令触发词
				return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-chart", map[string]bool{
					"skill": true,
				}), nil
			}
			return makeCommandRequest(ctx, parser.ModuleMusic, "music-chart"), nil
		},
	}, executeMusic)
}

// renderMusicChartMessage renders a chart through the normal render path: the
// render cache and, for allow-listed endpoints, the Drawing artifact directive.
func renderMusicChartMessage(rc *RequestContext, musicCtrl *rendermusic.Controller, query rendermusic.ChartQuery) (onebot11.Message, error) {
	if rc == nil || musicCtrl == nil {
		return nil, usererror.Misconfigured(errors.New("music chart renderer is not configured"))
	}
	payload, err := musicCtrl.BuildMusicChartRequest(query)
	if err != nil {
		if ids := rendermusic.ExtractAmbiguousMusicIDs(err); len(ids) > 1 {
			return renderAmbiguousMusicIDsMessages(rc, musicCtrl, query.Region, err, ids)
		}
		return nil, err
	}
	image, err := musicCtrl.RenderMusicChartRequestImage(payload)
	if err != nil {
		return nil, err
	}
	return rc.RenderedImageMessage(image)
}
