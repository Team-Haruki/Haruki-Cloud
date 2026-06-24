package handler

import (
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderinventory "haruki-cloud/internal/pjsk/render/inventory"
)

const inventoryListHelp = `使用方式:
/查背包
/查背包 水晶
/查背包 火罐
/查背包 ms材料
/查背包 记忆

空参数默认不展示水晶、火罐、MySekai 材料和记忆。国服不支持 ms材料、记忆。`

type inventoryListParams struct {
	userQueryParams
	Filter renderinventory.Filter `json:"filter,omitempty"`
}

func (sekaiHandlers) InventoryListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "inventory/list",
			Commands: []string{
				"/背包一览", "/查背包",
				"/pjsk inventory", "/inventory",
			},
			Helper: inventoryListHelp,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildInventoryListParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMisc, "inventory-list", params), nil
		},
	}, executeInventory)
}

func executeInventory(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.App.Inventory == nil {
		return nil, unsupportedModeError("inventory", "")
	}
	if rc.Cmd == nil || rc.Cmd.Mode != "inventory-list" {
		mode := ""
		if rc.Cmd != nil {
			mode = rc.Cmd.Mode
		}
		return nil, unsupportedModeError("inventory", mode)
	}

	params := inventoryListParams{}
	mergeParams(rc.Cmd.Params, &params)
	if err := validateInventoryFilterForRegion(rc.Region, params.Filter); err != nil {
		return nil, err
	}

	binding, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
	if suiteErr != nil {
		return nil, suiteErr
	}
	if suiteSnapshot == nil {
		return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}

	publicDetailedProfile, _ := resolveCommandDisplayProfiles(rc, suiteSnapshot)
	data, err := rc.App.Inventory.WithContext(rc.Ctx).RenderList(renderinventory.Query{
		Region:   rc.Region,
		Profile:  publicDetailedProfile,
		Snapshot: suiteSnapshot,
		Filter:   params.Filter,
	})
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func buildInventoryListParams(ctx HarrukiSekaiHandlerContext) (inventoryListParams, error) {
	self, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return inventoryListParams{}, err
	}
	filter, err := parseInventoryFilter(ctx.GetArgs(), ctx.originalTriggerCmd)
	if err != nil {
		return inventoryListParams{}, err
	}
	if err := validateInventoryFilterForRegion(ctx.Region(), filter); err != nil {
		return inventoryListParams{}, err
	}
	return inventoryListParams{
		userQueryParams: userQueryParams{
			Mode:           self.Mode,
			Platform:       self.Platform,
			PlatformUserID: self.PlatformUserID,
			Selector:       self.Selector,
		},
		Filter: filter,
	}, nil
}

func parseInventoryFilter(args string, trigger string) (renderinventory.Filter, error) {
	args = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(args)), ""))
	switch args {
	case "":
		return renderinventory.FilterDefault, nil
	case "水晶", "钻石", "石头", "彩石", "晶石":
		return renderinventory.FilterJewel, nil
	case "火罐", "演出能量", "体力", "能量":
		return renderinventory.FilterBoost, nil
	case "mysekai材料", "mysekai素材", "ms材料", "ms素材", "ms":
		return renderinventory.FilterMysekai, nil
	case "记忆", "回忆", "memoria", "memory":
		return renderinventory.FilterMemory, nil
	default:
		return renderinventory.FilterDefault, onebot11.NewReplayError(
			"未知的背包筛选参数：%s\n可用参数：水晶、火罐、ms材料、记忆\n使用方式：%s [水晶|火罐|ms材料|记忆]",
			strings.TrimSpace(args),
			trigger,
		)
	}
}

func validateInventoryFilterForRegion(region renderregion.Value, filter renderinventory.Filter) error {
	if renderregion.WithDefault(region) != renderregion.CN {
		return nil
	}
	switch filter {
	case renderinventory.FilterMysekai:
		return onebot11.NewReplayError("国服暂不支持查询 MySekai 材料")
	case renderinventory.FilterMemory:
		return onebot11.NewReplayError("国服暂不支持查询记忆")
	default:
		return nil
	}
}
