package handler

import (
	"fmt"

	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/render/gacha"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func executeGacha(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Gachas == nil {
		return nil, fmt.Errorf("gacha service unavailable: sekai client not configured")
	}
	gachaCtrl := rc.App.Gachas.WithContext(rc.Ctx)
	var data []byte
	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "gacha", "gacha-list":
		q := gacha.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = gachaCtrl.RenderGachaList(q)
	case "gacha-detail":
		q := gacha.DetailQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = gachaCtrl.RenderGachaDetail(q)
	default:
		return nil, unsupportedModeError("gacha", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}
