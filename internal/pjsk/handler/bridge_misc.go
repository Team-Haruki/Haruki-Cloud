package handler

import (
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/requestbuilder"
)

func executeMisc(rc *RequestContext) (message onebot11.Message, err error) {
	var data []byte
	switch rc.Cmd.Mode {
	case "misc-birthday":
		req := drawing.CharaBirthdayRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if req.Cid <= 0 || req.Month <= 0 || req.Day <= 0 || len(req.Cards) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildMiscBirthdayRequest(rc.Ctx, rc.Cmd, rc.App)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = rc.App.Misc.RenderCharaBirthday(req)
	default:
		return nil, unsupportedModeError("misc", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}
