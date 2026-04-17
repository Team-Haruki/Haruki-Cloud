package handler

import (
	"fmt"

	"haruki-cloud/internal/pjsk/onebot11"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/event"
)

func executeEvent(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Events == nil {
		return nil, fmt.Errorf("event service unavailable: sekai client not configured")
	}
	eventCtrl := rc.App.Events.WithContext(rc.Ctx)
	var data []byte
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "event-detail":
		q := event.DetailQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = eventCtrl.RenderEventDetail(q)
	case "event-list":
		q := event.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = eventCtrl.RenderEventList(q)
	case "event-record":
		req, buildErr := buildEventRecordFromSnapshot(rc, region)
		if buildErr != nil {
			return nil, buildErr
		}
		data, err = eventCtrl.RenderEventRecord(*req)
	default:
		return nil, unsupportedModeError("event", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}
