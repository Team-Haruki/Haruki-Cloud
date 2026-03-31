package handler

import (
	"fmt"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/vlive"
)

func executeVLive(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.VLive == nil {
		return nil, fmt.Errorf("vlive service unavailable: sekai client not configured")
	}
	query := vlive.ListQuery{Region: rc.Cmd.Region}
	mergeParams(rc.Cmd.Params, &query)
	text, err := rc.App.VLive.RenderText(query)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Text(text)}, nil
}
