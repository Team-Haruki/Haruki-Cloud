package sekai

// CustomMusicScorePublishedSearchResponse is returned by the game API when
// looking up a published custom music score by share ID.
type CustomMusicScorePublishedSearchResponse struct {
	UserCustomMusicScoreInfoJSON                     *UserCustomMusicScorePublishedResponse            `json:"userCustomMusicScoreInfoJson"`
	CustomMusicScoreOfficialCreatorPublishedResponse *CustomMusicScoreOfficialCreatorPublishedResponse `json:"customMusicScoreOfficialCreatorPublishedResponseJson"`
}

type UserCustomMusicScorePublishedResponse struct {
	UserCustomMusicScoreInfoJSON *UserCustomMusicScoreInfo `json:"userCustomMusicScoreInfoJson"`
	UserCustomMusicScoreID       string                    `json:"userCustomMusicScoreId"`
	UserID                       int64                     `json:"userId"`
	UserName                     string                    `json:"userName"`
	MusicID                      int                       `json:"musicId"`
	MusicDifficultyType          string                    `json:"musicDifficultyType"`
	PlayLevel                    int                       `json:"playLevel"`
	Description                  string                    `json:"description"`
	PreviewStartTimeSec          float64                   `json:"previewStartTimeSec"`
	PublishedAt                  int64                     `json:"publishedAt"`
	ReviewCount                  int                       `json:"reviewCount"`
	PlayCount                    int                       `json:"playCount"`
	FullComboRate                float64                   `json:"fullComboRate"`
}

type UserCustomMusicScoreInfo struct {
	MusicID                    int    `json:"musicId"`
	Title                      string `json:"title"`
	UserCustomMusicScorePath   string `json:"userCustomMusicScorePath"`
	BaseUserCustomMusicScoreID string `json:"baseUserCustomMusicScoreId"`
}

type CustomMusicScoreOfficialCreatorPublishedResponse struct {
	CustomMusicScoreID string  `json:"customMusicScoreId"`
	ReviewCount        int     `json:"reviewCount"`
	PlayCount          int     `json:"playCount"`
	FullComboRate      float64 `json:"fullComboRate"`
}
