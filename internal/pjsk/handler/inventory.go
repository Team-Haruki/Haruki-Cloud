package handler

import (
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderinventory "haruki-cloud/internal/pjsk/render/inventory"
	"haruki-cloud/utils/usererror"
)

type inventoryListParams struct {
	userQueryParams
	Filter renderinventory.Filter `json:"filter,omitempty"`
}

func (sekaiHandlers) InventoryListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "inventory/list",
		Commands: []string{
			"/背包一览", "/查背包", "/持有物", "/查持有物",
			"/pjsk inventory", "/inventory",
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
	if rc.Region == renderregion.CN && params.Filter == renderinventory.FilterMysekai {
		return rejectCNMySekai(rc)
	}
	if err := validateInventoryFilterForRegion(rc.Region, params.Filter); err != nil {
		return nil, err
	}

	rc.warmSuiteAndPublicProfile(false)
	binding, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
	if suiteErr != nil {
		return nil, suiteErr
	}
	if suiteSnapshot == nil {
		return nil, suiteDataNotFoundError(binding)
	}

	publicDetailedProfile, _ := resolveCommandDisplayProfiles(rc, suiteSnapshot)
	data, err := rc.App.Inventory.WithContext(rc.Ctx).RenderListImage(renderinventory.Query{
		Region:       rc.Region,
		Profile:      publicDetailedProfile,
		Snapshot:     suiteSnapshot,
		Filter:       params.Filter,
		MaterialRows: inventoryMaterialRows(rc),
	})
	if err != nil {
		return nil, err
	}
	return rc.RenderedImageMessage(data)
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
		Mode:           self.Mode,
		Platform:       self.Platform,
		PlatformUserID: self.PlatformUserID,
		Selector:       self.Selector,
		Filter:         filter,
	}, nil
}

func parseInventoryFilter(args string, trigger string) (renderinventory.Filter, error) {
	args = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(args)), ""))
	switch args {
	case "":
		return renderinventory.FilterDefault, nil
	case "水晶", "钻石", "石头", "彩石", "晶石": //copylint:ignore 解析关键字
		return renderinventory.FilterJewel, nil
	case "火罐", "演出能量", "体力", "能量": //copylint:ignore 解析关键字
		return renderinventory.FilterBoost, nil
	case "mysekai材料", "mysekai素材", "ms材料", "ms素材", "ms": //copylint:ignore 解析关键字
		return renderinventory.FilterMysekai, nil
	case "记忆", "回忆", "memoria", "memory": //copylint:ignore 解析关键字
		return renderinventory.FilterMemory, nil
	default:
		return renderinventory.FilterDefault, usererror.BadParam(strings.TrimSpace(args), i18n.M("inventory.filter_unknown"))
	}
}

func validateInventoryFilterForRegion(region renderregion.Value, filter renderinventory.Filter) error {
	if renderregion.WithDefault(region) != renderregion.CN {
		return nil
	}
	switch filter {
	case renderinventory.FilterMemory:
		return usererror.Invalid(i18n.M("inventory.memory_cn_unavailable", i18n.Data{"Region": i18n.RegionLabel("cn")}))
	default:
		return nil
	}
}

// inventoryMaterialRows is the region's generic master row store, which
// serves materials columns the typed query does not read (expiredAt).
func inventoryMaterialRows(rc *RequestContext) renderinventory.MaterialRowSource {
	if rc == nil || rc.App == nil {
		return nil
	}
	src := rc.App.ProviderForRegion(rc.Region)
	if src == nil {
		return nil
	}
	rows, _ := src.MySekai().(renderinventory.MaterialRowSource)
	return rows
}
