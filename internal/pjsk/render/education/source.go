package education

import renderregion "haruki-cloud/internal/pjsk/render/region"

type Source interface {
	DefaultRegion() renderregion.Value
	GetChallengeRewardsByCharacter(charID int) []*ChallengeReward
	GetResourceBoxByPurpose(purpose string, id int) *ResourceBox
}

type ChallengeReward struct {
	ID            int
	CharacterID   int
	HighScore     int
	ResourceBoxID int
}

type ResourceBox struct {
	ID                 int
	ResourceBoxPurpose string
	ResourceBoxType    string
	Description        string
	Details            []ResourceBoxDetail
}

type ResourceBoxDetail struct {
	ResourceType     string `json:"resourceType"`
	ResourceID       int    `json:"resourceId"`
	ResourceQuantity int    `json:"resourceQuantity"`
}
