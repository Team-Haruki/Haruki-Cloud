package userdata

import (
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

type LocalFileConfig struct {
	DefaultRegion renderregion.Value
	UserJSON      string
	MusicMetaJSON string
	MySekaiJSON   string
}

type Service struct {
	configured bool
	initErr    error

	baseProfile    *drawing.DetailedProfileCardRequest
	musicResult    map[string]map[int]string
	challenge      *ChallengeLiveData
	rawData        *RawUserData
	musicMetaBytes []byte
	rawFilePath    string
	musicMetaPath  string
	rawJSON        []byte
}
