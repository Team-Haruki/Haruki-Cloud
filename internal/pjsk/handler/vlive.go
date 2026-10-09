package handler

import (
	"errors"
	"fmt"
	"strings"

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
			return onebot11.Message{onebot11.Text("当前没有虚拟Live")}, nil
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
		return onebot11.Message{onebot11.Text("该区服暂无个人虚拟Live")}, nil
	case errors.Is(err, vlive.ErrSoloLiveNotFound):
		return onebot11.Message{onebot11.Text(fmt.Sprintf("未找到个人虚拟Live：%s\n不带参数使用 /vlive 查看虚拟Live列表", query.Query))}, nil
	case err != nil:
		return nil, err
	}
	return rc.RenderedImageMessage(data)
}
