package handler

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/card"
)

const searchSingleCardHelp = `查单张卡的方式:
1. 直接使用卡牌ID
2. 角色昵称+负数 代表角色新卡，例如 mnr-1 代表 mnr 最新一张卡
3. 直接使用负数代表全局倒序已上线卡，例如 -1 代表当前区服最新上线卡`

const searchMultiCardHelp = `查询多张卡牌的筛选参数:
角色昵称：miku
团/团oc/团vs/纯vs：mmj mmjoc mmjv 纯v
稀有度/属性/技能：4 四星 生日 蓝 蓝星 判 判卡 分 分卡 奶 奶卡 p分
限定类型：非限 限定 期间限定 fes cfes bfes 联动限定
年份：25年 去年
活动id或者箱活缩写：event123 mnr1
以上参数可以混合使用，用空格分隔`

const cardSearchHelp = searchSingleCardHelp + "\n\n" + searchMultiCardHelp

func (sekaiHandlers) CardDetailHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "card/detail",
			Commands: []string{
				"/card-detail", "/查卡", "/查牌", "/查卡牌", "/pjsk card",
			},
			Helper: cardSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return resolveCardDetailOrList(ctx, false)
		},
	}, executeCard)
}

func (sekaiHandlers) CardListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "card/list",
			Commands: []string{
				"/卡牌列表", "/cards", "/pjsk cards", "/card-list",
			},
			Helper: cardSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return resolveCardDetailOrList(ctx, true)
		},
	}, executeCard)
}

func resolveCardDetailOrList(ctx HarrukiSekaiHandlerContext, preferFilter bool) (*CommandRequest, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	if isCardBoxQuery(args) {
		ctx.SetArgs(cleanCardBoxArgs(args))
		params, err := newCardBoxParams(ctx, args, false)
		if err != nil {
			return nil, err
		}
		return makeCommandRequestWithParams(ctx, parser.ModuleCard, "card-box", params), nil
	}
	if preferFilter {
		params, err := newCardListParams(ctx, args, true)
		if err != nil {
			return nil, err
		}
		return makeCommandRequestWithParams(ctx, parser.ModuleCard, "card-list", params), nil
	}
	if looksLikeSingleCardQuery(args, preferFilter) {
		return makeCommandRequestWithParams(ctx, parser.ModuleCard, "card-detail", card.Query{Query: args, Region: ctx.Region().String()}), nil
	}
	params, err := newCardListParams(ctx, args, false)
	if err != nil {
		return nil, err
	}
	return makeCommandRequestWithParams(ctx, parser.ModuleCard, "card-list", params), nil
}

func looksLikeSingleCardQuery(args string, preferFilter bool) bool {
	if preferFilter {
		return card.LooksLikeSingleCardQueryPreferFilter(args)
	}
	return card.LooksLikeSingleCardQuery(args)
}

func (sekaiHandlers) CardBoxHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "card/box",
			Commands: []string{
				"/查箱", "/卡牌一览", "/卡面一览", "/卡一览", "/box", "/card-box", "/pjsk box",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			ctx.SetArgs(cleanCardBoxArgs(args))
			params, err := newCardBoxParams(ctx, args, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleCard, "card-box", params), nil
		},
	}, executeCard)
}

func isCardBoxQuery(args string) bool {
	lower := strings.ToLower(strings.TrimSpace(args))
	return strings.Contains(lower, " box") ||
		strings.HasSuffix(lower, "box") ||
		strings.Contains(lower, " id") ||
		strings.HasSuffix(lower, "id") ||
		strings.Contains(lower, " before") ||
		strings.HasSuffix(lower, "before")
}

func cardBoxParams(args string) map[string]any {
	lower := strings.ToLower(strings.TrimSpace(args))
	return map[string]any{
		"show_id":            strings.Contains(lower, "id"),
		"show_box":           strings.Contains(lower, "box"),
		"use_after_training": !strings.Contains(lower, "before"),
	}
}

func newCardListParams(ctx HarrukiSekaiHandlerContext, args string, strictFilterOnly bool) (map[string]any, error) {
	params, err := newSelfQueryParamsMap(ctx)
	if err != nil {
		return nil, err
	}
	params["query"] = args
	params["region"] = ctx.Region().String()
	if strictFilterOnly {
		params["strict_filter_only"] = true
	}
	return params, nil
}

func newCardBoxParams(ctx HarrukiSekaiHandlerContext, args string, strictFilterOnly bool) (map[string]any, error) {
	params, err := newSelfQueryParamsMap(ctx)
	if err != nil {
		return nil, err
	}
	for key, value := range cardBoxParams(args) {
		params[key] = value
	}
	if strictFilterOnly {
		params["strict_filter_only"] = true
	}
	return params, nil
}

func cleanCardBoxArgs(args string) string {
	replacer := strings.NewReplacer("id", "", "box", "", "before", "")
	return strings.TrimSpace(replacer.Replace(strings.ToLower(args)))
}

func (sekaiHandlers) CardImgHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "card/image",
			Commands: []string{
				"/pjsk card img",
				"/查卡面", "/卡面原图", "/卡面", "/card", "/卡图",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, errors.New("请输入要查询的卡牌")
			}
			return makeCommandRequest(ctx, parser.ModuleCard, "card-image"), nil
		},
	}, executeCard)
}

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
		q.DetailedProfile = rc.GetDetailedProfile()
		data, err = cardCtrl.RenderCardList(q)
	case "card-box":
		q := card.Query{
			Query:            rc.Cmd.Query,
			Region:           rc.Cmd.Region,
			UseAfterTraining: commandBoolPtr(true),
			Title:            resolveCardCatalogTitle(rc),
			DetailedProfile:  resolveCardBoxDetailedProfile(rc),
		}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		if (q.ShowBox || strings.TrimSpace(q.Query) == "") && !hasCardCatalogOwnedData(q.DetailedProfile) {
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

func hasCardCatalogOwnedData(detail *drawing.DetailedProfileCardRequest) bool {
	return detail != nil && len(detail.UserCards) > 0
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
