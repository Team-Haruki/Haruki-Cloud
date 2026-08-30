package handler

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

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
团/团oc/团vs/纯vs：mmj mmjoc mmjv 纯v/原v
稀有度/属性/技能：4 四星 生日 蓝 蓝星 判 判卡 分 分卡 奶 奶卡 p分
限定类型：非限 限定 期间限定 fes cfes bfes 联动限定
年份：25年 去年
活动id或者箱活缩写：event123 mnr1
以上参数可以混合使用，用空格分隔`

const cardSearchHelp = searchSingleCardHelp + "\n\n" + searchMultiCardHelp

func (sekaiHandlers) CardDetailHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "card/detail",
		Commands: []string{
			"/card-detail", "/查卡", "/查牌", "/查卡牌", "/pjsk card",
		},
		Helper: cardSearchHelp,
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return resolveCardDetailOrList(ctx, false)
		},
	}, executeCard)
}

func (sekaiHandlers) CardListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "card/list",
		Commands: []string{
			"/卡牌列表", "/cards", "/pjsk cards", "/card-list",
		},
		Helper: cardSearchHelp,
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
		return makeCommandRequestWithParams(ctx, parser.ModuleCard, cardBoxCommand, params), nil
	}
	if preferFilter {
		params, err := newCardListParams(ctx, args, true)
		if err != nil {
			return nil, err
		}
		return makeCommandRequestWithParams(ctx, parser.ModuleCard, cardListCommand, params), nil
	}
	if looksLikeSingleCardQuery(args, preferFilter) {
		return makeCommandRequestWithParams(ctx, parser.ModuleCard, "card-detail", card.Query{Query: args, Region: ctx.Region().String()}), nil
	}
	params, err := newCardListParams(ctx, args, false)
	if err != nil {
		return nil, err
	}
	return makeCommandRequestWithParams(ctx, parser.ModuleCard, cardListCommand, params), nil
}

func looksLikeSingleCardQuery(args string, preferFilter bool) bool {
	if preferFilter {
		return card.LooksLikeSingleCardQueryPreferFilter(args)
	}
	return card.LooksLikeSingleCardQuery(args)
}

func (sekaiHandlers) CardBoxHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "card/box",
		Commands: []string{
			"/查箱", "/卡牌一览", "/卡面一览", "/卡一览", "/box", "/card-box", "/pjsk box",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			ctx.SetArgs(cleanCardBoxArgs(args))
			params, err := newCardBoxParams(ctx, args, true)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleCard, cardBoxCommand, params), nil
		},
	}, executeCard)
}

func isCardBoxQuery(args string) bool {
	lower := strings.ToLower(strings.TrimSpace(args))
	return hasCardBoxControlToken(lower, "box") ||
		hasCardBoxControlToken(lower, "id") ||
		hasCardBoxControlToken(lower, "before") ||
		hasCardBoxUnownedToken(lower)
}

func cardBoxParams(args string) map[string]any {
	lower := strings.ToLower(strings.TrimSpace(args))
	params := map[string]any{
		"show_id":            hasCardBoxControlToken(lower, "id"),
		"show_box":           hasCardBoxControlToken(lower, "box"),
		"unowned_only":       hasCardBoxUnownedToken(lower),
		"use_after_training": !hasCardBoxControlToken(lower, "before"),
	}
	if groupBy := cardBoxGroupBy(args); groupBy != "" {
		params["group_by"] = groupBy
	}
	return params
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
	lower := strings.ToLower(strings.ReplaceAll(args, "属性", " "))
	tokens := strings.FieldsFunc(lower, isCardBoxTokenSeparator)
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		switch token {
		case "id", "box", "before", "attr", "attrs", "attribute", "attributes", "未持有", "未拥有", "unowned", "missing", "miss":
			continue
		default:
			kept = append(kept, token)
		}
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

func cardBoxGroupBy(args string) string {
	lower := strings.ToLower(strings.TrimSpace(args))
	if strings.Contains(args, "属性") ||
		hasCardBoxControlToken(lower, "attr") ||
		hasCardBoxControlToken(lower, "attrs") ||
		hasCardBoxControlToken(lower, "attribute") ||
		hasCardBoxControlToken(lower, "attributes") {
		return card.CardBoxGroupByAttr
	}
	return ""
}

func hasCardBoxUnownedToken(text string) bool {
	return hasCardBoxControlToken(text, "未持有") ||
		hasCardBoxControlToken(text, "未拥有") ||
		hasCardBoxControlToken(text, "unowned") ||
		hasCardBoxControlToken(text, "missing") ||
		hasCardBoxControlToken(text, "miss")
}

func hasCardBoxControlToken(text string, token string) bool {
	for _, item := range strings.FieldsFunc(text, isCardBoxTokenSeparator) {
		if item == token {
			return true
		}
	}
	return false
}

func isCardBoxTokenSeparator(ch rune) bool {
	return unicode.IsSpace(ch) || strings.ContainsRune(",，/\\+-_|｜", ch)
}

func (sekaiHandlers) CardImgHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "card/image",
		Commands: []string{
			"/pjsk card img",
			"/查卡面", "/卡面原图", "/卡面", "/card", "/卡图",
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
	defer func() {
		region := ""
		query := ""
		if rc != nil {
			region = rc.RegionStr
			if rc.Cmd != nil {
				query = rc.Cmd.Query
			}
		}
		err = normalizeCardUserFacingErrorForLookup(err, region, query)
	}()

	if rc.App.Cards == nil {
		return nil, fmt.Errorf("card service unavailable: sekai client not configured")
	}
	cardCtrl := rc.App.Cards.WithContext(rc.Ctx)
	buildDoneText := func(summary string) string {
		if strings.TrimSpace(summary) == "" {
			return ""
		}
		return fmt.Sprintf("已处理%s。", summary)
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "card-detail":
		q := card.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		q.AllowUnreleased = allowReadOnlyLeaks(q.Region)
		data, err = cardCtrl.RenderCardDetail(q)
		if err != nil {
			return nil, err
		}
		return rc.ImageMessage(data)
	case cardListCommand:
		q := card.ListRequest{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		q.AllowUnreleased = allowReadOnlyLeaks(q.Region)
		q.DetailedProfile, _ = resolveCommandDisplayProfiles(rc, rc.ResolveSnapshot(false))
		data, err = cardCtrl.RenderCardList(q)
		if err != nil {
			return nil, err
		}
		image, imageErr := rc.ImageMessage(data)
		if imageErr != nil {
			return nil, imageErr
		}
		if !cardCtrl.ShouldShowSummaryForList(q) {
			return image, nil
		}
		return append(onebot11.Message{onebot11.Text(buildDoneText(cardCtrl.SummaryForList(q)))}, image...), nil
	case cardBoxCommand:
		q := card.Query{
			Query:            rc.Cmd.Query,
			Region:           rc.Cmd.Region,
			UseAfterTraining: commandBoolPtr(true),
			Title:            resolveCardCatalogTitle(rc),
			DetailedProfile:  resolveCardBoxDetailedProfile(rc),
		}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		if (q.ShowBox || q.UnownedOnly || strings.TrimSpace(q.Query) == "") && !hasCardCatalogOwnedData(q.DetailedProfile) {
			detail, detailErr := requireCardCatalogDetailedProfile(rc)
			if detailErr != nil {
				return nil, detailErr
			}
			q.DetailedProfile = detail
		}
		queries := []card.Query{q}
		data, err = cardCtrl.RenderCardBox(queries)
		if err != nil {
			return nil, err
		}
		image, imageErr := rc.ImageMessage(data)
		if imageErr != nil {
			return nil, imageErr
		}
		if !cardCtrl.ShouldShowSummaryForBox(q) {
			return image, nil
		}
		return append(onebot11.Message{onebot11.Text(buildDoneText(cardCtrl.SummaryForBox(q)))}, image...), nil
	case "card-image":
		q := card.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		q.AllowUnreleased = allowReadOnlyLeaks(q.Region)
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
		return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}
	snap := rc.ResolveSnapshot(false)
	if snap == nil {
		if snapshotErr := rc.SnapshotError(false); snapshotErr != nil {
			return nil, normalizeToolboxDataFetchError(snapshotErr, "suite", binding)
		}
		return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}
	detail := snap.DetailedProfile(rc.Region)
	if detail == nil || len(detail.UserCards) == 0 {
		return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}
	return cloneDetailedProfileForCurrentTarget(rc, detail), nil
}
