package sekai

import (
	"errors"
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strconv"
	"strings"
)

func (sekaiHandlers) CardDetailHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/detail",
			Commands: []string{
				"/card-detail", "/详情", "/查卡",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if isCardBoxQuery(args) {
				ctx.SetArgs(cleanCardBoxArgs(args))
				return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-box", cardBoxParams(args)), nil
			}
			if isSingleCardIDQuery(args) {
				return makeResolvedCmd(ctx, parser.ModuleCard, "card-detail"), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleCard, "card-list"), nil
		},
	}
}

func (sekaiHandlers) CardListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/list",
			Commands: []string{
				"/查牌", "/查卡牌", "/卡牌列表", "/card", "/cards", "/pjsk card", "/pjsk member",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if isCardBoxQuery(args) {
				ctx.SetArgs(cleanCardBoxArgs(args))
				return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-box", cardBoxParams(args)), nil
			}
			if isSingleCardIDQuery(args) {
				return makeResolvedCmd(ctx, parser.ModuleCard, "card-detail"), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleCard, "card-list"), nil
		},
	}
}

func (sekaiHandlers) CardBoxHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "card/box",
			Commands: []string{
				"/查箱", "/查框", "/卡牌一览", "/卡面一览", "/卡一览", "/box", "/card-box", "/pjsk box",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			ctx.SetArgs(cleanCardBoxArgs(args))
			return makeResolvedCmdWithParams(ctx, parser.ModuleCard, "card-box", cardBoxParams(args)), nil
		},
	}
}

func isSingleCardIDQuery(args string) bool {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return false
	}
	value, err := strconv.Atoi(fields[0])
	return err == nil && value > 0
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
				"/查卡面", "/卡面原图", "/卡面",
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
