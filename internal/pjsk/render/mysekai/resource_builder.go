package mysekai

import (
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	phenomIcons := c.loadIconNameMap("mysekaiPhenomenas.json", "iconAssetbundleName")
	return &drawing.MysekaiResourceRequest{
		Profile:             *profile,
		Phenoms:             extractMysekaiPhenoms(region, func(p string) string { return c.regionPath(region, p) }, phenomIcons, merged),
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
	if name := c.resolveGateSkinAssetbundleName(gateSkinID); name != "" {
		return name
	}
	if gateID <= 0 {
		return ""
	}
	return stringValue(c.masterdata.loadMapByID("mysekaiGates.json")[gateID]["assetbundleName"])
}

func (c *Controller) resolveGateSkinAssetbundleName(gateSkinID int) string {
	if gateSkinID <= 0 {
		return ""
	}
	skin := c.masterdata.loadMapByID("mysekaiGateSkins.json")[gateSkinID]
	skinTypeID := intNumber(skin["mysekaiGateSkinTypeId"], 0)
	if skinTypeID <= 0 {
		return ""
	}
	filename := gateSkinMasterdataFilename(stringValue(skin["mysekaiGateSkinType"]))
	if filename == "" {
		return ""
	}
	return stringValue(c.masterdata.loadMapByID(filename)[skinTypeID]["assetbundleName"])
}

func gateSkinMasterdataFilename(skinType string) string {
	switch skinType {
	case "unit":
		return "mysekaiGateUnitSkins.json"
	case "common":
		return "mysekaiGateCommonSkins.json"
	default:
		return ""
	}
}

// RenderResource renders the MySekai resource view.
func (c *Controller) RenderResource(query ResourceQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildResourceRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateMysekaiResource(payload)
}
