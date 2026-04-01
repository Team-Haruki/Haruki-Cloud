package handler

import (
	"fmt"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/card"
)

func executeCard(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App.Cards == nil {
		return nil, fmt.Errorf("card service unavailable: sekai client not configured")
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "card-detail":
		q := card.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = rc.App.Cards.RenderCardDetail(q)
	case "card-list":
		q := card.ListRequest{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.DetailedProfile = rc.GetDetailedProfile()
		data, err = rc.App.Cards.RenderCardList(q)
	case "card-box":
		useAfterTraining := true
		q := card.Query{
			Query:            rc.Cmd.Query,
			Region:           rc.Cmd.Region,
			UseAfterTraining: &useAfterTraining,
			DetailedProfile:  resolveCardBoxDetailedProfile(rc.Ctx, rc.Cmd, rc.App),
		}
		mergeParams(rc.Cmd.Params, &q)
		queries := []card.Query{q}
		data, err = rc.App.Cards.RenderCardBox(queries)
	case "card-image":
		q := card.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := rc.App.Cards.ResolveCardImages(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		message = make(onebot11.Message, 0, len(result.Paths))
		for _, path := range result.Paths {
			image, imageErr := assetImageMessage(path, rc.App, BotModulePJSK)
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
		return nil, fmt.Errorf("bridge: unsupported card mode %q", rc.Cmd.Mode)
	}
	if err != nil {
		return
	}
	return rc.ImageMessage(data)
}
