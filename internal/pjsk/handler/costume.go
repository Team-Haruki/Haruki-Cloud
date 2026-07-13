package handler

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
)

const costumeSearchHelp = `服装查询:
1. /查服装 1 mnr 颜色2 查询服装详情；颜色省略时为原色1
2. /查头饰 2003001 miku ln 颜色3 查询头饰详情；旧短ID会转为候选饰品列表
3. /查服装、/查头饰、/查发型 可按本区服组件名称查询；追加“角色23”时直接进入详情
4. /服装列表 服装/饰品/发型 查询分类
5. /饰品列表 或 /发型列表 miku ln 是快捷入口；发型ID按角色从1开始
6. 可组合筛选: 男装 女装 男饰品 女饰品 男发型 女发型 角色昵称 关键词 p2 每页480 全部
7. /组合 角色2 服装797 颜色1 饰品797001 颜色1 发型1 临时 3D 试穿
8. 角色可写1到31或精确昵称；Miku须加团队（vs/mmj/ln/vbs/wxs/n25）`

func (sekaiHandlers) CostumeDetailHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "costume/detail",
			Commands: []string{
				"/查服装", "/查衣装", "/costume", "/pjsk costume", "/查饰品", "/查头饰", "/accessory",
				"/查发型",
			},
			Helper: costumeSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			partType := costumeDetailPartTypeForTrigger(ctx.GetTriggerCmd())
			lookupQuery, lookupOK, lookupErr := rendercostume.ParseLookupQuery(args, partType)
			if lookupErr == nil && lookupOK {
				return makeCostumeDetailCommandRequest(ctx, lookupQuery), nil
			}
			if isCostumeNameSearchTrigger(ctx.GetTriggerCmd()) {
				if query, ok, err := rendercostume.ParseNamedLookupQuery(args, partType); err != nil {
					return nil, err
				} else if ok {
					return makeCostumeDetailCommandRequest(ctx, query), nil
				}
			}
			if lookupErr != nil {
				return nil, lookupErr
			}
			return makeCostumeListCommandRequest(ctx, partType), nil
		},
	}, executeCostume)
}

func (sekaiHandlers) CostumeListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "costume/list",
			Commands: []string{
				"/服装列表", "/衣装列表", "/costumes", "/pjsk costumes",
				"/饰品列表", "/查饰品列表", "/accessories",
				"/发型列表", "/查发型列表", "/hairstyles",
			},
			Helper: costumeSearchHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			partType := costumeListPartTypeForTrigger(ctx.GetTriggerCmd())
			if isCostumeNameSearchTrigger(ctx.GetTriggerCmd()) {
				if query, ok, err := rendercostume.ParseNamedLookupQuery(strings.TrimSpace(ctx.GetArgs()), partType); err != nil {
					return nil, err
				} else if ok {
					return makeCostumeDetailCommandRequest(ctx, query), nil
				}
			}
			return makeCostumeListCommandRequest(ctx, partType), nil
		},
	}, executeCostume)
}

func makeCostumeDetailCommandRequest(ctx HarrukiSekaiHandlerContext, query rendercostume.Query) *CommandRequest {
	query.Region = ctx.Region().String()
	request := makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-detail", query)
	request.Query = query.Query
	return request
}

func makeCostumeListCommandRequest(ctx HarrukiSekaiHandlerContext, partType string) *CommandRequest {
	args := strings.TrimSpace(ctx.GetArgs())
	query := rendercostume.ListQuery{
		Query:    args,
		Region:   ctx.Region().String(),
		PartType: partType,
	}
	if isCostumeNameSearchTrigger(ctx.GetTriggerCmd()) {
		query.Query = ""
		query.Keyword = args
	}
	request := makeCommandRequestWithParams(ctx, parser.ModuleCostume, "costume-list", query)
	request.Query = query.Query
	return request
}

func isCostumeNameSearchTrigger(trigger string) bool {
	switch strings.ToLower(strings.TrimSpace(trigger)) {
	case "/查服装", "/查衣装", "/查饰品", "/查头饰", "/查发型":
		return true
	default:
		return false
	}
}

func costumeDetailPartTypeForTrigger(trigger string) string {
	if partType := costumeListPartTypeForTrigger(trigger); partType != "" {
		return partType
	}
	return "body"
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
	case strings.Contains(trigger, "饰品"), strings.Contains(trigger, "头饰"), strings.Contains(trigger, "accessor"):
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
			if listQuery, ok := legacyAccessoryListQuery(err, q); ok {
				return renderCostumeList(rc, costumeCtrl, listQuery)
			}
			return nil, normalizeCostume3DError(err)
		}
		return rc.ImageMessage(data)
	case "costume-list":
		q := rendercostume.ListQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Region = rc.Cmd.Region
		return renderCostumeList(rc, costumeCtrl, q)
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

func legacyAccessoryListQuery(err error, detail rendercostume.Query) (rendercostume.ListQuery, bool) {
	var legacyErr *rendercostume.LegacyAccessoryIDError
	if !errors.As(err, &legacyErr) || len(legacyErr.AccessoryIDs) == 0 {
		return rendercostume.ListQuery{}, false
	}
	return rendercostume.ListQuery{
		Region:        detail.Region,
		PartType:      "head",
		Character3DID: detail.Character3DID,
		AccessoryIDs:  append([]int(nil), legacyErr.AccessoryIDs...),
	}, true
}

func renderCostumeList(rc *RequestContext, costumeCtrl *rendercostume.Controller, query rendercostume.ListQuery) (onebot11.Message, error) {
	data, payload, err := costumeCtrl.RenderCostumeListWithRequest(query)
	if err != nil {
		return nil, normalizeCostume3DError(err)
	}
	image, err := rc.ImageMessage(data)
	if err != nil {
		return nil, err
	}
	prompt := rendercostume.BuildListPrompt(payload)
	if strings.TrimSpace(prompt) == "" {
		return image, nil
	}
	return append(onebot11.Message{onebot11.Text(prompt)}, image...), nil
}
