package mysekai

import (
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

// BuildInfoPanelRequest builds the standalone info panel for MySekai data: the
// profile card every MySekai render carries, with only the MySekai source — or,
// with IncludeSuite, both sources as the shop and talk-list renders show them.
func (c *Controller) BuildInfoPanelRequest(query InfoPanelQuery) (*drawing.ProfileCardRequest, error) {
	c = c.withRegion(query.Region)
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}
	profile := c.mysekaiProfileCard(region, merged, query.Profile, query.IncludeSuite)
	if profile == nil {
		return nil, fmt.Errorf("mysekai info panel requires profile data")
	}
	return profile, nil
}

func (c *Controller) RenderInfoPanelImage(query InfoPanelQuery) (drawing.ImageResult, error) {
	if c == nil || c.drawing == nil {
		return drawing.ImageResult{}, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	payload, err := c.BuildInfoPanelRequest(query)
	finishBuild()
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateInfoPanelImage(payload)
}
