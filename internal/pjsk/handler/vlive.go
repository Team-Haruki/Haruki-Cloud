package handler

import (
	"errors"
	"fmt"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/vlive"
)

func (sekaiHandlers) LiveHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path:     "vlive",
			Commands: []string{"/pjsk live", "/虚拟live", "/pjsk vlive", "/vlive"},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return makeCommandRequest(ctx, parser.ModuleVLive, "vlive-list"), nil
		},
	}, executeVLive)
}

func executeVLive(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.VLive == nil {
		return nil, fmt.Errorf("vlive service unavailable: sekai client not configured")
	}
	query := vlive.ListQuery{
		Region:   rc.Cmd.Region,
		TimeZone: resolveRequesterHarukiUserTimeZone(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID),
	}
	mergeParams(rc.Cmd.Params, &query)
	data, err := rc.App.VLive.WithContext(rc.Ctx).RenderList(query)
	if err != nil {
		if errors.Is(err, vlive.ErrNoLives) {
			return onebot11.Message{onebot11.Text("当前没有虚拟Live")}, nil
		}
		return nil, err
	}
	return rc.ImageMessage(data)
}
