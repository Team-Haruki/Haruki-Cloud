package sekai

// GetInformationResponse is the response returned by GetInformation.
type GetInformationResponse struct {
	Informations []InformationEntry `json:"informations"`
}

// InformationEntry is one in-game announcement or news item.
type InformationEntry struct {
	ID                    int    `json:"id"`
	Seq                   int    `json:"seq"`
	DisplayOrder          int    `json:"displayOrder"`
	InformationType       string `json:"informationType"`
	InformationTag        string `json:"informationTag"`
	BrowseType            string `json:"browseType"`
	Platform              string `json:"platform"`
	Title                 string `json:"title"`
	Path                  string `json:"path"`
	StartAt               int64  `json:"startAt"`
	BannerAssetbundleName string `json:"bannerAssetbundleName,omitempty"`
	Channels              string `json:"channels,omitempty"`
	EndAt                 int64  `json:"endAt,omitempty"`
}
