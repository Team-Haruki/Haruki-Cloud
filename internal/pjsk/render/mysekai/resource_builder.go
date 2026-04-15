package mysekai

import (
	"fmt"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

// BuildResourceRequest builds the request for rendering MySekai resource view.
func (c *Controller) BuildResourceRequest(query ResourceQuery) (*drawing.MysekaiResourceRequest, error) {
	c = c.withRegion(query.Region)
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	profile := c.mysekaiProfileCard(region, merged, query.Profile, false)
	if profile == nil {
		return nil, fmt.Errorf("mysekai resource requires profile data")
	}

	gateID, gateLevel, gateSkinID := extractMysekaiGateInfo(merged)
	return &drawing.MysekaiResourceRequest{
		Profile:             *profile,
		Phenoms:             extractMysekaiPhenoms(region, func(p string) string { return c.regionPath(region, p) }, merged),
		GateID:              gateID,
		GateLevel:           gateLevel,
		GateIconPath:        c.resolveGateIconPath(region, gateID, gateSkinID),
		VisitCharacters:     c.extractVisitCharacters(region, merged),
		SiteResourceNumbers: c.extractSiteResourceNumbers(region, merged),
	}, nil
}

// resolveGateIconPath returns the path to the gate icon image.
func (c *Controller) resolveGateIconPath(region renderregion.Value, gateID, gateSkinID int) string {
	if assetbundleName := c.resolveGateAssetbundleName(gateID, gateSkinID); assetbundleName != "" {
		return c.regionPath(region, fmt.Sprintf("mysekai/thumbnail/gate_large/%s.png", assetbundleName))
	}
	return c.staticPath(fmt.Sprintf("mysekai/gate_icon/gate_%d.png", gateID))
}

// resolveGateAssetbundleName returns the asset bundle name for a gate.
func (c *Controller) resolveGateAssetbundleName(gateID, gateSkinID int) string {
	if gateSkinID > 0 {
		skins := c.masterdata.loadMapByID("mysekaiGateSkins.json")
		skin := skins[gateSkinID]
		if len(skin) > 0 {
			skinType := stringValue(skin["mysekaiGateSkinType"])
			skinTypeID := intNumber(skin["mysekaiGateSkinTypeId"], 0)
			if skinTypeID > 0 {
				switch skinType {
				case "unit":
					if unitSkin := c.masterdata.loadMapByID("mysekaiGateUnitSkins.json")[skinTypeID]; len(unitSkin) > 0 {
						if name := stringValue(unitSkin["assetbundleName"]); name != "" {
							return name
						}
					}
				case "common":
					if commonSkin := c.masterdata.loadMapByID("mysekaiGateCommonSkins.json")[skinTypeID]; len(commonSkin) > 0 {
						if name := stringValue(commonSkin["assetbundleName"]); name != "" {
							return name
						}
					}
				}
			}
		}
	}

	if gateID <= 0 {
		return ""
	}
	gates := c.masterdata.loadMapByID("mysekaiGates.json")
	gate := gates[gateID]
	if len(gate) == 0 {
		return ""
	}
	return stringValue(gate["assetbundleName"])
}

// RenderResource renders the MySekai resource view.
func (c *Controller) RenderResource(query ResourceQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildResourceRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiResource(payload)
}
