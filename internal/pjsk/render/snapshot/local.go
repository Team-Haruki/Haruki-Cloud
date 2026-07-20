package snapshot

import (
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	musicMetaView  *meta.View
	rawFilePath    string
	musicMetaPath  string
	rawJSON        []byte
}
