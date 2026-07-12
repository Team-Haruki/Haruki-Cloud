package drawing

type CostumeColorVariant struct {
	CostumeID        int     `json:"costume_id"`
	ColorID          int     `json:"color_id"`
	ColorName        string  `json:"color_name"`
	AssetBundleName  string  `json:"asset_bundle_name"`
	ThumbnailPath    string  `json:"thumbnail_path"`
	PreviewImagePath *string `json:"preview_image_path,omitempty"`
	SourceCardIDs    []int   `json:"source_card_ids,omitempty"`
}

type CostumeBasic struct {
	CostumeID          int                   `json:"costume_id"`
	CostumeGroupID     int                   `json:"costume_group_id"`
	OutfitID           int                   `json:"outfit_id,omitempty"`
	AccessoryID        int                   `json:"accessory_id,omitempty"`
	Character3DID      int                   `json:"character_3d_id,omitempty"`
	Character3DIDs     []int                 `json:"character_3d_ids,omitempty"`
	Name               string                `json:"name"`
	PartType           string                `json:"part_type"`
	PartName           string                `json:"part_name"`
	Costume3DType      string                `json:"costume_3d_type"`
	CharacterID        int                   `json:"character_id"`
	CharacterName      string                `json:"character_name"`
	CharacterGender    string                `json:"character_gender,omitempty"`
	Rarity             string                `json:"rarity,omitempty"`
	HowToObtain        string                `json:"how_to_obtain,omitempty"`
	Designer           string                `json:"designer,omitempty"`
	AssetBundleName    string                `json:"asset_bundle_name,omitempty"`
	ColorID            int                   `json:"color_id,omitempty"`
	ColorName          string                `json:"color_name,omitempty"`
	PublishedAt        int64                 `json:"published_at,omitempty"`
	ArchivePublishedAt int64                 `json:"archive_published_at,omitempty"`
	ThumbnailPath      string                `json:"thumbnail_path"`
	PreviewImagePath   *string               `json:"preview_image_path,omitempty"`
	SourceCardIDs      []int                 `json:"source_card_ids,omitempty"`
	Variants           []CostumeColorVariant `json:"variants,omitempty"`
}

type CostumeListRequest struct {
	Region      string         `json:"region"`
	Title       *string        `json:"title,omitempty"`
	Page        int            `json:"page"`
	PageSize    int            `json:"page_size"`
	Total       int            `json:"total"`
	TotalPages  int            `json:"total_pages"`
	FilterLabel string         `json:"filter_label,omitempty"`
	Costumes    []CostumeBasic `json:"costumes"`
}

type CostumeDetailRequest struct {
	Region            string       `json:"region"`
	Costume           CostumeBasic `json:"costume"`
	CharacterIconPath string       `json:"character_icon_path,omitempty"`
	UnitLogoPath      string       `json:"unit_logo_path,omitempty"`
}
