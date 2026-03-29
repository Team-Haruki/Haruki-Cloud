package sekai

import (
	"errors"
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/card"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strings"
)

const searchSingleCardHelp = `查单张卡的方式:
1. 直接使用卡牌ID
2. 角色昵称+负数 代表角色新卡，例如 mnr-1 代表 mnr 最新一张卡`

const searchMultiCardHelp = `查询多张卡牌的筛选参数:
角色昵称：miku
团/团oc/团vs/纯vs：mmj mmjoc mmjv 纯v
稀有度/属性/技能：4 四星 生日 蓝 蓝星 判 分 p分
限定类型：非限 限定 期间限定 fes
年份：25年 去年
活动id或者箱活缩写：event123 mnr1
以上参数可以混合使用，用空格分隔`

const cardSearchHelp = searchSingleCardHelp + "\n\n" + searchMultiCardHelp

func (sekaiHandlers) CardDetailHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/detail",
			Commands: []string{
				"/card-detail", "/查卡", "/查牌", "/查卡牌", "/pjsk card",
			},
			Helper: cardSearchHelp,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return resolveCardDetailOrList(ctx), nil
		},
	}
}

func (sekaiHandlers) CardListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/list",
			Commands: []string{
				"/卡牌列表", "/cards", "/pjsk cards", "/card-list",
			},
			Helper: cardSearchHelp,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return resolveCardDetailOrList(ctx), nil
		},
	}
}

func resolveCardDetailOrList(ctx SekaiHandlerContext) *parser.ResolvedCommand {
	args := strings.TrimSpace(ctx.GetArgs())
	if isCardBoxQuery(args) {
		ctx.SetArgs(cleanCardBoxArgs(args))
		return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-box", cardBoxParams(args))
	}
	if card.LooksLikeSingleCardQuery(args) {
		return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-detail", card.Query{Query: args, Region: ctx.Region().String()})
	}
	return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-list", card.ListRequest{Query: args, Region: ctx.Region().String()})
}

func (sekaiHandlers) CardBoxHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/box",
			Commands: []string{
				"/查箱", "/卡牌一览", "/卡面一览", "/卡一览", "/box", "/card-box", "/pjsk box",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			ctx.SetArgs(cleanCardBoxArgs(args))
			return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-box", cardBoxParams(args)), nil
		},
	}
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

func cleanCardBoxArgs(args string) string {
	replacer := strings.NewReplacer("id", "", "box", "", "before", "")
	return strings.TrimSpace(replacer.Replace(strings.ToLower(args)))
}

func (sekaiHandlers) CardImgHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/image",
			Commands: []string{
				"/pjsk card img",
				"/查卡面", "/卡面原图", "/卡面", "/card", "/卡图",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, errors.New("请输入要查询的卡牌")
			}
			return makeResolvedCmd(ctx, parser.ModuleCard, "card-image"), nil
		},
	}
}

func (sekaiHandlers) CardStoryHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk card story",
				"/卡牌剧情", "/卡面剧情", "/卡剧情", "/卡牌故事", "/卡面故事", "/卡故事",
			},
			Disabled: true,
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if args == "" {
				return nil, errors.New("请输入要查询的卡牌")
			}

			refresh := false
			save := true
			if strings.Contains(args, "refresh") {
				args = strings.TrimSpace(strings.ReplaceAll(args, "refresh", ""))
				refresh = true
			}

			model := ""
			if strings.Contains(args, "model:") {
				parts := strings.SplitN(args, "model:", 2)
				args = strings.TrimSpace(parts[0])
				model = strings.TrimSpace(parts[1])
				refresh = true
				save = false
			}

			return nil, fmt.Errorf(
				"TODO: 卡牌剧情查询未实现，query=%q, refresh=%t, save=%t, model=%q",
				args, refresh, save, model,
			)
		},
	}
}

func (sekaiHandlers) BoxHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/box",
			Commands: []string{
				"/pjsk box",
				"/卡牌一览", "/卡面一览", "/卡一览",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())

			showID := false
			if strings.Contains(args, "id") {
				showID = true
				args = strings.TrimSpace(strings.ReplaceAll(args, "id", ""))
			}

			showBox := false
			if strings.Contains(args, "box") {
				showBox = true
				args = strings.TrimSpace(strings.ReplaceAll(args, "box", ""))
			}

			useAfterTraining := true
			if strings.Contains(args, "before") {
				useAfterTraining = false
				args = strings.TrimSpace(strings.ReplaceAll(args, "before", ""))
			}

			ctx.SetArgs(strings.TrimSpace(args))
			return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-box", map[string]any{
				"show_id":            showID,
				"show_box":           showBox,
				"use_after_training": useAfterTraining,
			}), nil
		},
	}
}
