package education

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

var areaFilterUnitAreaIDs = map[string]int{
	"light_sound":    5,
	"idol":           7,
	"street":         8,
	"theme_park":     9,
	"school_refusal": 10,
}

var piaproCharacterIDs = map[int]struct{}{
	21: {},
	22: {},
	23: {},
	24: {},
	25: {},
	26: {},
}

var powerBonusUnitOrder = []string{
	"light_sound",
	"idol",
	"street",
	"theme_park",
	"school_refusal",
	"piapro",
}

var powerBonusAttrOrder = []string{
	"cute",
	"cool",
	"pure",
	"happy",
	"mysterious",
}

var gateUnitByID = map[int]string{
	1: "light_sound",
	2: "idol",
	3: "street",
	4: "theme_park",
	5: "school_refusal",
}

func (c *Controller) resolveSnapshotContext(
	region renderregion.Value,
	profile *drawing.DetailedProfileCardRequest,
	snapshot snapshot.Snapshot,
) (*resolvedSnapshotContext, error) {
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("education controller is not initialized")
	}

	if snapshot == nil {
		snapshot = c.snapshot
	}
	if snapshot == nil {
		return nil, fmt.Errorf("local user snapshot is not configured")
	}
	if err := snapshot.Require(); err != nil {
		return nil, err
	}

	resolvedRegion := c.sources.ResolveRegion(region)
	source, ok := c.sources.SourceForRegion(resolvedRegion)
	if !ok {
		return nil, fmt.Errorf("education data source is not configured")
	}

	raw := snapshot.RawData()
	if raw == nil {
		return nil, fmt.Errorf("user snapshot is missing raw suite data")
	}

	if profile == nil {
		profile = snapshot.DetailedProfile(resolvedRegion)
	}
	if profile == nil {
		return nil, fmt.Errorf("user snapshot is missing profile data")
	}

	return &resolvedSnapshotContext{
		region:   resolvedRegion,
		source:   source,
		snapshot: snapshot,
		raw:      raw,
		profile:  profile,
	}, nil
}
