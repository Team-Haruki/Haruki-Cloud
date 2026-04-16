package handler

import (
	"fmt"

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
		if _, bindErr := rc.requireBinding(); bindErr != nil {
			return nil, bindErr
		}
		q := card.ListRequest{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		q.Title = resolveCardCatalogTitle(rc)
		q.DetailedProfile = rc.GetDetailedProfile()
		data, err = cardCtrl.RenderCardList(q)
	case "card-box":
		if _, bindErr := rc.requireBinding(); bindErr != nil {
			return nil, bindErr
		}
		useAfterTraining := true
		q := card.Query{
			Query:            rc.Cmd.Query,
			Region:           rc.Cmd.Region,
			UseAfterTraining: &useAfterTraining,
			Title:            resolveCardCatalogTitle(rc),
			DetailedProfile:  resolveCardBoxDetailedProfile(rc),
		}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
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
