package handler

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
)

func executeMysekaiHousingSK(rc *RequestContext, region string) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.App.MySekai == nil {
		return nil, fmt.Errorf("mysekai service unavailable: mysekai controller is not configured")
	}
	if rc.App.SekaiAPI == nil {
		return nil, fmt.Errorf("sekai api client is not configured")
	}

	query := rendermysekai.HousingCompetitionLineQuery{Region: region}
	mergeParams(rc.Cmd.Params, &query)
	if strings.TrimSpace(query.Region) == "" {
		query.Region = regionWithDefault(region)
	}

	apiClient := rc.App.SekaiAPI.WithContext(rc.Ctx)
	controller := rc.App.MySekai.WithContext(rc.Ctx)
	result, err := controller.BuildHousingCompetitionLine(rc.Ctx, apiClient, query)
	if err != nil {
		return nil, err
	}
	lineImage, err := controller.RenderHousingCompetitionLine(result)
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, lineImage, rc.App, BotModulePJSK)
}
