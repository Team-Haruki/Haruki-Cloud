package handler

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/render/card"
)

func executeCard(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Cards == nil {
		return nil, fmt.Errorf("card service unavailable: sekai client not configured")
	}
	cardCtrl := rc.App.Cards.WithContext(rc.Ctx)
	var data []byte
	switch rc.Cmd.Mode {
	case "card-detail":
		q := card.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		data, err = cardCtrl.RenderCardDetail(q)
	case "card-list":
		q := card.ListRequest{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		q.Title = resolveCardCatalogTitle(rc)
		q.DetailedProfile = rc.GetDetailedProfile()
		data, err = cardCtrl.RenderCardList(q)
	case "card-box":
		q := card.Query{
			Query:            rc.Cmd.Query,
			Region:           rc.Cmd.Region,
			UseAfterTraining: new(true),
			Title:            resolveCardCatalogTitle(rc),
			DetailedProfile:  resolveCardBoxDetailedProfile(rc),
		}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		if strings.TrimSpace(q.Query) == "" {
			detail, detailErr := requireCardCatalogDetailedProfile(rc)
			if detailErr != nil {
				return nil, detailErr
			}
			q.DetailedProfile = detail
		}
		queries := []card.Query{q}
		data, err = cardCtrl.RenderCardBox(queries)
	case "card-image":
		q := card.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		result, resolveErr := cardCtrl.ResolveCardImages(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		message = make(onebot11.Message, 0, len(result.Paths))
		for _, path := range result.Paths {
			image, imageErr := assetImageMessage(rc.Ctx, path, rc.App, BotModulePJSK)
			if imageErr != nil {
				return nil, imageErr
			}
			message = append(message, image...)
		}
		if len(message) == 0 {
			return nil, fmt.Errorf("bridge: card %d did not resolve any images", result.Card.ID)
		}
		return message, nil
	default:
		return nil, unsupportedModeError("card", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func requireCardCatalogDetailedProfile(rc *RequestContext) (*drawing.DetailedProfileCardRequest, error) {
	if rc == nil {
		return nil, onebot11.NewReplayError(ErrMsgCardCatalogRequiresSuite)
	}
	binding, _ := rc.GetBinding()
	if binding == nil {
		if rc.bindingErr != nil {
			return nil, rc.bindingErr
		}
		return nil, accountdata.ErrNoBinding
	}
	if !binding.SuiteVisible {
		return nil, onebot11.NewReplayError(ErrMsgCardCatalogRequiresSuite)
	}
	snap := rc.ResolveSnapshot(false)
	if snap == nil {
		return nil, onebot11.NewReplayError(ErrMsgCardCatalogRequiresSuite)
	}
	detail := snap.DetailedProfile(rc.Region)
	if detail == nil || len(detail.UserCards) == 0 {
		return nil, onebot11.NewReplayError(ErrMsgCardCatalogRequiresSuite)
	}
	return detail, nil
}
