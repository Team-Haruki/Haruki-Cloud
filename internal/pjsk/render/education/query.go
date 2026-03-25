package education

import (
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

type ChallengeLiveQuery struct {
	Region  renderregion.Value                  `json:"region"`
	Profile *drawing.DetailedProfileCardRequest `json:"-"`
}
