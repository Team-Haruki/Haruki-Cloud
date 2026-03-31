package handler

import (
	"fmt"

	"haruki-cloud/api/bot/onebot11"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/stamp"
)

func executeStamp(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Stamps == nil {
		return nil, fmt.Errorf("stamp service unavailable: sekai client not configured")
	}
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "stamp-list":
		q := stamp.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		if q.All {
			images, renderErr := rc.App.Stamps.RenderStampListPages(q)
			if renderErr != nil {
				return nil, renderErr
			}
			message = make(onebot11.Message, 0, len(images))
			for _, img := range images {
				segment, imageErr := imageMessage(img, rc.App, BotModulePJSK)
				if imageErr != nil {
					return nil, imageErr
				}
				message = append(message, segment...)
			}
			if len(message) == 0 {
				return nil, fmt.Errorf("stamp all mode did not produce any images")
			}
			return message, nil
		}
		data, renderErr := rc.App.Stamps.RenderStampList(q)
		if renderErr != nil {
			return nil, renderErr
		}
		return imageMessage(data, rc.App, BotModulePJSK)
	default:
		return nil, fmt.Errorf("bridge: unsupported stamp mode %q", rc.Cmd.Mode)
	}
	return nil, fmt.Errorf("bridge: unsupported stamp mode %q", rc.Cmd.Mode)
}
