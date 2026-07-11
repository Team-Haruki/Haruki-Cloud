package handler

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
)

const costumeSearchHelp = `服装查询:
1. /查服装 12345 查询单个服装详情
2. /服装列表 服装/饰品/发型 查询分类
3. /饰品列表 或 /发型列表 是快捷入口
4. 可组合筛选: 男装 女装 男饰品 女饰品 男发型 女发型 角色昵称 关键词 p2 每页480 全部
5. /组合 服装33001 发型33021 饰品30129 临时 3D 试穿`

func (sekaiHandlers) CostumeDetailHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "costume/detail",
			Commands: []string{
				"/查服装", "/查衣装", "/costume", "/pjsk costume",
			},
			Helper: costumeSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if id, ok := rendercostume.ParseExplicitCostumeID(args); ok {
				return makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-detail", rendercostume.Query{
					Query:  args,
					ID:     id,
					Region: ctx.Region().String(),
				}), nil
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-list", rendercostume.ListQuery{
				Query:  args,
				Region: ctx.Region().String(),
			}), nil
		},
	}, executeCostume)
}

func (sekaiHandlers) CostumeListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "costume/list",
			Commands: []string{
				"/服装列表", "/衣装列表", "/costumes", "/pjsk costumes",
				"/饰品列表", "/查饰品", "/查饰品列表", "/accessories",
				"/发型列表", "/查发型", "/查发型列表", "/hairstyles",
			},
			Helper: costumeSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			partType := costumeListPartTypeForTrigger(ctx.GetTriggerCmd())
			return makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-list", rendercostume.ListQuery{
				Query:    strings.TrimSpace(ctx.GetArgs()),
				Region:   ctx.Region().String(),
				PartType: partType,
			}), nil
		},
	}, executeCostume)
}

func (sekaiHandlers) CostumeComboHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "costume/combo",
			Commands: []string{
				"/组合", "/试穿", "/3d试穿", "/pjsk combo",
			},
			Helper: costumeSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-combo", rendercostume.ComboQuery{
				Query:  strings.TrimSpace(ctx.GetArgs()),
				Region: ctx.Region().String(),
			}), nil
		},
	}, executeCostume)
}

func costumeListPartTypeForTrigger(trigger string) string {
	trigger = strings.ToLower(strings.TrimSpace(trigger))
	switch {
	case strings.Contains(trigger, "饰品"), strings.Contains(trigger, "accessor"):
		return "head"
	case strings.Contains(trigger, "发型"), strings.Contains(trigger, "hairstyle"):
		return "hair"
	default:
		return ""
	}
}

func executeCostume(rc *RequestContext) (onebot11.Message, error) {
	if rc.App.Costumes == nil {
		return nil, fmt.Errorf("costume service unavailable: sekai client not configured")
	}
	costumeCtrl := rc.App.Costumes.WithContext(rc.Ctx)
	switch rc.Cmd.Mode {
	case "costume-detail":
		q := rendercostume.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		data, err := costumeCtrl.RenderCostumeDetail(q)
		if err != nil {
			return nil, err
		}
		return rc.ImageMessage(data)
	case "costume-list":
		q := rendercostume.ListQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		data, payload, err := costumeCtrl.RenderCostumeListWithRequest(q)
		if err != nil {
			return nil, err
		}
		image, imageErr := rc.ImageMessage(data)
		if imageErr != nil {
			return nil, imageErr
		}
		prompt := rendercostume.BuildListPrompt(payload)
		if strings.TrimSpace(prompt) == "" {
			return image, nil
		}
		return append(onebot11.Message{onebot11.Text(prompt)}, image...), nil
	case "costume-combo":
		q := rendercostume.ComboQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		data, err := costumeCtrl.RenderCostumeCombo(q)
		if err != nil {
			return nil, normalizeCostume3DError(err)
		}
		return rc.ImageMessage(data)
	default:
		return nil, unsupportedModeError("costume", rc.Cmd.Mode)
	}
}
