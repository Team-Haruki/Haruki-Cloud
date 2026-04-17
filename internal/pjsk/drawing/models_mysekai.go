package drawing

// =========================== MySekai Models ===========================

type MysekaiPhenomRequest struct {
	RefreshReason  string `json:"refresh_reason"`
	ImagePath      string `json:"image_path"`
	BackgroundFill []int  `json:"background_fill"` // Color tuple
	Text           string `json:"text"`
	TextFill       []int  `json:"text_fill"`
}

type MysekaiVisitCharacter struct {
	SdImagePath         string  `json:"sd_image_path"`
	MemoriaImagePath    *string `json:"memoria_image_path,omitempty"`
	IsRead              bool    `json:"is_read"`
	IsReservation       bool    `json:"is_reservation"`
	ReservationIconPath *string `json:"reservation_icon_path,omitempty"`
}

type MysekaiSiteResourceNumber struct {
	ImagePath       string                  `json:"image_path"`
	ResourceNumbers []MysekaiResourceNumber `json:"resource_numbers"`
}

type MysekaiResourceNumber struct {
	ImagePath           string  `json:"image_path"`
	Number              int     `json:"number"`
	TextColor           []int   `json:"text_color"`
	HasMusicRecord      bool    `json:"has_music_record"`
	MusicRecordIconPath *string `json:"music_record_icon_path,omitempty"`
}

type MysekaiResourceRequest struct {
	Profile             ProfileCardRequest          `json:"profile"`
	BackgroundImagePath *string                     `json:"background_image_path,omitempty"`
	Phenoms             []MysekaiPhenomRequest      `json:"phenoms"`
	GateID              int                         `json:"gate_id"`
	GateLevel           int                         `json:"gate_level"`
	GateIconPath        string                      `json:"gate_icon_path"`
	VisitCharacters     []MysekaiVisitCharacter     `json:"visit_characters"`
	SiteResourceNumbers []MysekaiSiteResourceNumber `json:"site_resource_numbers,omitempty"`
}

type MysekaiMsrMapSiteInfo struct {
	ImagePath string  `json:"image_path"`
	GridSize  float64 `json:"grid_size"`
	OffsetX   float64 `json:"offset_x,omitempty"`
	OffsetZ   float64 `json:"offset_z,omitempty"`
	DirX      float64 `json:"dir_x,omitempty"`
	DirZ      float64 `json:"dir_z,omitempty"`
	RevXZ     bool    `json:"rev_xz,omitempty"`
	Scale     float64 `json:"scale,omitempty"`
	CropBbox  []int   `json:"crop_bbox,omitempty"`
}

type MysekaiMsrMapHarvestPoint struct {
	ID                *int    `json:"id,omitempty"`
	ImagePath         string  `json:"image_path"`
	FallbackImagePath *string `json:"fallback_image_path,omitempty"`
	PositionX         float64 `json:"position_x"`
	PositionZ         float64 `json:"position_z"`
	Status            string  `json:"status,omitempty"`
	Size              *int    `json:"size,omitempty"`
	OffsetX           float64 `json:"offset_x,omitempty"`
	OffsetZ           float64 `json:"offset_z,omitempty"`
	Alpha             float64 `json:"alpha,omitempty"`
}

type MysekaiMsrMapResourceDrop struct {
	ID                  int     `json:"id"`
	Type                string  `json:"type"`
	ImagePath           string  `json:"image_path"`
	PositionX           float64 `json:"position_x"`
	PositionZ           float64 `json:"position_z"`
	Quantity            int     `json:"quantity,omitempty"`
	Status              string  `json:"status,omitempty"`
	SmallIcon           *bool   `json:"small_icon,omitempty"`
	Hide                bool    `json:"hide,omitempty"`
	Rarity              int     `json:"rarity,omitempty"`
	AttachmentImagePath *string `json:"attachment_image_path,omitempty"`
	OutlineColor        []int   `json:"outline_color,omitempty"`
	OutlineWidth        *int    `json:"outline_width,omitempty"`
	LightSize           *int    `json:"light_size,omitempty"`
}

type MysekaiMsrMapData struct {
	MapID         int                         `json:"map_id"`
	Site          MysekaiMsrMapSiteInfo       `json:"site"`
	HarvestPoints []MysekaiMsrMapHarvestPoint `json:"harvest_points,omitempty"`
	ResourceDrops []MysekaiMsrMapResourceDrop `json:"resource_drops,omitempty"`
}

type MysekaiMsrMapRequest struct {
	Maps                 []MysekaiMsrMapData `json:"maps"`
	ShowHarvested        bool                `json:"show_harvested"`
	PhenomenaGroundColor []int               `json:"phenomena_ground_color,omitempty"`
	SpawnPositionX       float64             `json:"spawn_position_x,omitempty"`
	SpawnPositionZ       float64             `json:"spawn_position_z,omitempty"`
	SpawnImagePath       *string             `json:"spawn_image_path,omitempty"`
	SpawnSize            int                 `json:"spawn_size,omitempty"`
	RareLightImagePath   *string             `json:"rare_light_image_path,omitempty"`
	LargeIconSize        int                 `json:"large_icon_size,omitempty"`
	SmallIconSize        int                 `json:"small_icon_size,omitempty"`
	IconZOffset          int                 `json:"icon_zoffset,omitempty"`
	DrawBgFill           []int               `json:"draw_bg_fill,omitempty"`
}

type MysekaiFixture struct {
	ID            int     `json:"id"`
	ImagePath     string  `json:"image_path"`
	CharacterID   *int    `json:"character_id,omitempty"`
	CharaIconPath *string `json:"chara_icon_path,omitempty"`
	Obtained      bool    `json:"obtained"`
}

type MysekaiFixtureSubGenre struct {
	Name            *string          `json:"name,omitempty"`
	ImagePath       *string          `json:"image_path,omitempty"`
	ProgressMessage *string          `json:"progress_message,omitempty"`
	Fixtures        []MysekaiFixture `json:"fixtures"`
}

type MysekaiFixtureMainGenre struct {
	Name            string                   `json:"name"`
	ImagePath       string                   `json:"image_path"`
	ProgressMessage *string                  `json:"progress_message,omitempty"`
	SubGenres       []MysekaiFixtureSubGenre `json:"sub_genres"`
}

type MysekaiFixtureListRequest struct {
	Profile         *ProfileCardRequest       `json:"profile,omitempty"`
	ProgressMessage *string                   `json:"progress_message,omitempty"`
	ShowID          bool                      `json:"show_id"`
	MainGenres      []MysekaiFixtureMainGenre `json:"main_genres"`
}

type MysekaiFixtureColorImage struct {
	ImagePath string  `json:"image_path"`
	ColorCode *string `json:"color_code,omitempty"`
}

type MysekaiFixtureMaterial struct {
	ImagePath string `json:"image_path"`
	Quantity  int    `json:"quantity"`
}

type MysekaiReactionCharacterGroups struct {
	Number                int        `json:"number"`
	CharacterUintIDGroups [][]int    `json:"character_uint_id_groups,omitempty"`
	CharaIconPathGroups   [][]string `json:"chara_icon_path_groups,omitempty"`
}

type MysekaiFixtureDetailRequest struct {
	Title                   string                           `json:"title"`
	Images                  []MysekaiFixtureColorImage       `json:"images"`
	MainGenreName           string                           `json:"main_genre_name"`
	MainGenreImagePath      string                           `json:"main_genre_image_path"`
	SubGenreName            *string                          `json:"sub_genre_name,omitempty"`
	SubGenreImagePath       *string                          `json:"sub_genre_image_path,omitempty"`
	Size                    map[string]int                   `json:"size"` // width, depth, height
	FirstPutCost            int                              `json:"first_put_cost"`
	SecondPutCost           int                              `json:"second_put_cost"`
	BasicInfo               []string                         `json:"basic_info,omitempty"`
	CostMaterials           []MysekaiFixtureMaterial         `json:"cost_materials,omitempty"`
	RecycleMaterials        []MysekaiFixtureMaterial         `json:"recycle_materials,omitempty"`
	ReactionCharacterGroups []MysekaiReactionCharacterGroups `json:"reaction_character_groups,omitempty"`
	Tags                    []string                         `json:"tags,omitempty"`
	Friendcodes             []string                         `json:"friendcodes,omitempty"`
	FriendcodeSource        string                           `json:"friendcode_source"`
}

type MysekaiGateMaterialItem struct {
	ImagePath   string `json:"image_path"`
	Quantity    int    `json:"quantity"`
	Color       []int  `json:"color"`
	SumQuantity string `json:"sum_quantity"`
}

type MysekaiGateLevelMaterials struct {
	Level int                       `json:"level"`
	Color []int                     `json:"color"`
	Items []MysekaiGateMaterialItem `json:"items"`
}

type MysekaiGateMaterials struct {
	ID             int                         `json:"id"`
	Level          *int                        `json:"level,omitempty"`
	LevelMaterials []MysekaiGateLevelMaterials `json:"level_materials"`
}

type MysekaiDoorUpgradeRequest struct {
	Profile       *ProfileCardRequest    `json:"profile,omitempty"`
	GateMaterials []MysekaiGateMaterials `json:"gate_materials"`
}

type MysekaiMusicrecord struct {
	ID        *int   `json:"id,omitempty"`
	ImagePath string `json:"image_path"`
	Obtained  bool   `json:"obtained"`
}

type MysekaiCategoryMusicrecord struct {
	Tag             string               `json:"tag"`
	TagIconPath     string               `json:"tag_icon_path"`
	ProgressMessage *string              `json:"progress_message,omitempty"`
	Musicrecords    []MysekaiMusicrecord `json:"musicrecords"`
}

type MysekaiMusicrecordRequest struct {
	Profile              ProfileCardRequest           `json:"profile"`
	ProgressMessage      *string                      `json:"progress_message,omitempty"`
	CategoryMusicrecords []MysekaiCategoryMusicrecord `json:"category_musicrecords"`
}

type MysekaiTalkFixtures struct {
	Fixtures            []MysekaiFixture `json:"fixtures"`
	NoreadNum           int              `json:"noread_num"`
	CharacterIDs        [][]int          `json:"character_ids,omitempty"`
	CharaIconPathGroups [][]string       `json:"chara_icon_path_groups,omitempty"`
}

type MysekaiSingleTalkMainGenre struct {
	Name      string                  `json:"name"`
	ImagePath string                  `json:"image_path"`
	SubGenres [][]MysekaiTalkFixtures `json:"sub_genres"`
}

type MysekaiTalkListRequest struct {
	Profile          *ProfileCardRequest          `json:"profile,omitempty"`
	SdImagePath      string                       `json:"sd_image_path"`
	ProgressMessage  *string                      `json:"progress_message,omitempty"`
	PromptMessage    *string                      `json:"prompt_message,omitempty"`
	ShowID           bool                         `json:"show_id"`
	SingleMainGenres []MysekaiSingleTalkMainGenre `json:"single_main_genres"`
	MultiReads       []MysekaiTalkFixtures        `json:"multi_reads"`
}
