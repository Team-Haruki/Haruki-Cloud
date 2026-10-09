package handler

import (
	"errors"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/vlive"
	"haruki-cloud/utils/usererror"
)

func (sekaiHandlers) LiveHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:     "vlive",
		Commands: []string{"/pjsk live", "/虚拟live", "/pjsk vlive", "/vlive"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return makeCommandRequest(ctx, parser.ModuleVLive, "vlive-list"), nil
		},
	}, executeVLive)
}

func executeVLive(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.VLive == nil {
		return nil, usererror.Misconfigured(errors.New("vlive service unavailable: sekai client not configured"))
	}
	timeZone := resolveRequesterHarukiUserTimeZone(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID)
	if vlive.IsDetailQuery(rc.Cmd.Query) {
		return executeVLiveDetail(rc, timeZone)
	}
	query := vlive.ListQuery{
		Region:   rc.Cmd.Region,
		TimeZone: timeZone,
	}
	mergeParams(rc.Cmd.Params, &query)
	data, err := rc.App.VLive.WithContext(rc.Ctx).RenderListImage(query)
	if err != nil {
		if errors.Is(err, vlive.ErrNoLives) {
			return onebot11.Message{onebot11.Text(i18n.T("vlive.none"))}, nil
		}
		return nil, err
	}
	return rc.RenderedImageMessage(data)
}

func executeVLiveDetail(rc *RequestContext, timeZone string) (onebot11.Message, error) {
	query := vlive.DetailQuery{
		Region:   rc.Cmd.Region,
		TimeZone: timeZone,
		Query:    strings.TrimSpace(rc.Cmd.Query),
	}
	data, err := rc.App.VLive.WithContext(rc.Ctx).RenderDetailImage(query)
	switch {
	case errors.Is(err, vlive.ErrNoSoloLives):
		return onebot11.Message{onebot11.Text(i18n.T("vlive.solo.none", i18n.Data{"Region": i18n.RegionLabel(regionWithDefault(rc.Cmd.Region))}))}, nil
	case errors.Is(err, vlive.ErrSoloLiveNotFound):
		return onebot11.Message{onebot11.Text(i18n.T("vlive.solo.not_found", i18n.Data{"Query": i18n.EchoQuery(query.Query)}))}, nil
	case err != nil:
		return nil, err
	}
	return rc.RenderedImageMessage(data)
}
