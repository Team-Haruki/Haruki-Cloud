package userdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
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

type RawUserData struct {
	Now                                               int64                    `json:"now"`
	UserGamedata                                      RawUserGamedata          `json:"userGamedata"`
	UserProfile                                       RawUserProfile           `json:"userProfile"`
	UserDecks                                         []RawUserDeck            `json:"userDecks"`
	UserCards                                         []RawUserCard            `json:"userCards"`
	UserMusicStats                                    []RawMusicResult         `json:"userMusicResults"`
	UserChallengeLiveSoloResults                      []RawChallengeLiveResult `json:"userChallengeLiveSoloResults"`
	UserChallengeLiveSoloStages                       []RawChallengeLiveStage  `json:"userChallengeLiveSoloStages"`
	UserChallengeLiveSoloHighScoreRewards             []RawChallengeLiveReward `json:"userChallengeLiveSoloHighScoreRewards"`
	UserCharacters                                    []RawUserCharacter       `json:"userCharacters"`
	UserAreas                                         []RawUserArea            `json:"userAreas"`
	UserMaterials                                     []RawUserMaterial        `json:"userMaterials"`
	UserMysekaiGates                                  []RawUserMysekaiGate     `json:"userMysekaiGates"`
	UserMysekaiFixtureGameCharacterPerformanceBonuses []RawUserFixtureBonus    `json:"userMysekaiFixtureGameCharacterPerformanceBonuses"`
	UserMusicClear                                    []RawMusicClear          `json:"userMusicDifficultyClearCounts"`
	UserHonors                                        []RawUserHonor           `json:"userHonors"`
	UserProfileHonors                                 []RawUserProfileHonor    `json:"userProfileHonors"`
	UserFrames                                        []RawUserFrame           `json:"userPlayerFrames"`
	UserEvents                                        []RawUserEvent           `json:"userEvents"`
	UserEventResults                                  []RawUserEventResult     `json:"userEventResults"`
	UserWorldBlooms                                   []RawUserWorldBloom      `json:"userWorldBlooms"`
}

type RawUserGamedata struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	Deck   int    `json:"deck"`
	Rank   int    `json:"rank"`
	Coin   int    `json:"coin"`
}

type RawUserProfile struct {
	ProfileImageType string `json:"profileImageType"`
	Word             string `json:"word"`
	TwitterID        string `json:"twitterId"`
}

type RawUserDeck struct {
	DeckID    int `json:"deckId"`
	Leader    int `json:"leader"`
	SubLeader int `json:"subLeader"`
	Member1   int `json:"member1"`
	Member2   int `json:"member2"`
	Member3   int `json:"member3"`
	Member4   int `json:"member4"`
	Member5   int `json:"member5"`
}

type RawUserCardEpisode struct {
	CardEpisodeID  int    `json:"cardEpisodeId"`
	ScenarioStatus string `json:"scenarioStatus"`
}

type RawUserCard struct {
	CardID                int                  `json:"cardId"`
	Level                 int                  `json:"level"`
	MasterRank            int                  `json:"masterRank"`
	SpecialTrainingStatus string               `json:"specialTrainingStatus"`
	DefaultImage          string               `json:"defaultImage"`
	Episodes              []RawUserCardEpisode `json:"episodes"`
}

type RawMusicResult struct {
	MusicID             int    `json:"musicId"`
	MusicDifficulty     string `json:"musicDifficulty"`
	MusicDifficultyType string `json:"musicDifficultyType"`
	PlayResult          string `json:"playResult"`
	FullComboFlg        bool   `json:"fullComboFlg"`
	FullPerfectFlg      bool   `json:"fullPerfectFlg"`
}

type RawChallengeLiveResult struct {
	CharacterID int `json:"characterId"`
	HighScore   int `json:"highScore"`
}

type RawChallengeLiveStage struct {
	CharacterID int `json:"characterId"`
	Rank        int `json:"rank"`
}

type RawChallengeLiveReward struct {
	ChallengeLiveHighScoreRewardID int `json:"challengeLiveHighScoreRewardId"`
	CharacterID                    int `json:"characterId"`
}

type RawUserCharacter struct {
	CharacterID   int `json:"characterId"`
	CharacterRank int `json:"characterRank"`
}

type RawUserArea struct {
	AreaItems []RawUserAreaItem `json:"areaItems"`
}

type RawUserAreaItem struct {
	AreaItemID int `json:"areaItemId"`
	Level      int `json:"level"`
}

type RawUserMaterial struct {
	MaterialID int `json:"materialId"`
	Quantity   int `json:"quantity"`
}

type RawUserMysekaiGate struct {
	MysekaiGateID    int `json:"mysekaiGateId"`
	MysekaiGateLevel int `json:"mysekaiGateLevel"`
}

type RawUserFixtureBonus struct {
	GameCharacterID int     `json:"gameCharacterId"`
	TotalBonusRate  float64 `json:"totalBonusRate"`
}

type RawMusicClear struct {
	MusicDifficultyType string `json:"musicDifficultyType"`
	LiveClear           int    `json:"liveClear"`
	FullCombo           int    `json:"fullCombo"`
	AllPerfect          int    `json:"allPerfect"`
}

type RawUserEvent struct {
	EventID    int `json:"eventId"`
	EventPoint int `json:"eventPoint"`
}

type RawUserEventResult struct {
	EventID int `json:"eventId"`
	Rank    int `json:"rank"`
}

type RawUserWorldBloom struct {
	EventID                int `json:"eventId"`
	GameCharacterID        int `json:"gameCharacterId"`
	WorldBloomChapterPoint int `json:"worldBloomChapterPoint"`
	Rank                   int `json:"rank"`
}

type RawUserHonor struct {
	Seq           int    `json:"seq"`
	HonorID       int    `json:"honorId"`
	HonorLevel    int    `json:"level"`
	ProfilePlayer bool   `json:"profilePlayer"`
	HonorRarity   string `json:"honorRarity"`
}

type RawUserFrame struct {
	PlayerFrameID           int    `json:"playerFrameId"`
	PlayerFrameAttachStatus string `json:"playerFrameAttachStatus"`
}

type RawUserProfileHonor struct {
	Seq              int    `json:"seq"`
	ProfileHonorType string `json:"profileHonorType"`
	HonorID          int    `json:"honorId"`
	HonorLevel       int    `json:"honorLevel"`
	HonorId2         int    `json:"honorId2"`
	BondsHonorWordId int    `json:"bondsHonorWordId"`
}

type ChallengeLiveData struct {
	Results []ChallengeLiveResult
	Stages  []ChallengeLiveStage
	Rewards []ChallengeLiveReward
}

type ChallengeLiveResult struct {
	CharacterID int
	HighScore   int
}

type ChallengeLiveStage struct {
	CharacterID int
	Rank        int
}

type ChallengeLiveReward struct {
	RewardID    int
	CharacterID int
}

func NewLocalFileService(sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper, cfg LocalFileConfig) *Service {
	service := &Service{
		configured:  strings.TrimSpace(cfg.UserJSON) != "",
		musicResult: make(map[string]map[int]string),
	}
	if !service.configured {
		return service
	}

	defaultRegion := renderregion.WithDefault(cfg.DefaultRegion)

	data, err := os.ReadFile(filepath.Clean(cfg.UserJSON))
	if err != nil {
		service.initErr = fmt.Errorf("read user snapshot: %w", err)
		return service
	}
	data, err = normalizeSnapshotJSON(data)
	if err != nil {
		service.initErr = err
		return service
	}

	if strings.TrimSpace(cfg.MySekaiJSON) != "" {
		data, err = mergeMySekaiJSON(data, cfg.MySekaiJSON)
		if err != nil {
			service.initErr = err
			return service
		}
	}

	var raw RawUserData
	if err := json.Unmarshal(data, &raw); err != nil {
		service.initErr = fmt.Errorf("decode user snapshot: %w", err)
		return service
	}
	if raw.UserGamedata.UserID == 0 {
		service.initErr = fmt.Errorf("user snapshot is missing userId")
		return service
	}

	activeDeck := FindActiveDeck(raw.UserDecks, raw.UserGamedata.Deck)
	leaderCardID := activeDeck.Leader
	leaderCard := FindUserCard(raw.UserCards, leaderCardID)
	leaderImagePath := resolveLeaderImagePath(sekaiClient, assetHelper, defaultRegion, leaderCardID, isAfterTraining(leaderCard))
	if leaderImagePath == "" {
		leaderImagePath = fallbackLeaderImagePath(assetHelper)
	}

	mode := strings.TrimSpace(raw.UserProfile.ProfileImageType)
	service.baseProfile = &drawing.DetailedProfileCardRequest{
		ID:              strconv.FormatInt(raw.UserGamedata.UserID, 10),
		Region:          strings.ToUpper(defaultRegion.String()),
		Nickname:        raw.UserGamedata.Name,
		Source:          "suite_dump",
		UpdateTime:      raw.Now,
		Mode:            common.OptionalString(mode),
		IsHideUID:       true,
		LeaderImagePath: leaderImagePath,
		HasFrame:        false,
		UserCards:       buildUserCardEntries(raw.UserCards),
	}
	service.musicResult = buildMusicResultMap(raw.UserMusicStats)
	service.challenge = &ChallengeLiveData{
		Results: convertChallengeResults(raw.UserChallengeLiveSoloResults),
		Stages:  convertChallengeStages(raw.UserChallengeLiveSoloStages),
		Rewards: convertChallengeRewards(raw.UserChallengeLiveSoloHighScoreRewards),
	}
	service.rawData = &raw
	service.rawJSON = data
	service.rawFilePath, err = writeNormalizedSnapshotFile("haruki-pjsk-user-*.json", data)
	if err != nil {
		service.initErr = err
		return service
	}

	if strings.TrimSpace(cfg.MusicMetaJSON) != "" {
		musicMetaBytes, err := os.ReadFile(filepath.Clean(cfg.MusicMetaJSON))
		if err != nil {
			service.initErr = fmt.Errorf("read music meta snapshot: %w", err)
			return service
		}
		service.musicMetaBytes = meta.InjectOmakase(musicMetaBytes)
		service.musicMetaPath = filepath.Clean(cfg.MusicMetaJSON)
	}

	return service
}

func (s *Service) Configured() bool {
	return s != nil && s.configured
}

func (s *Service) Require() error {
	if s == nil || !s.configured {
		return fmt.Errorf("local user snapshot is not configured")
	}
	if s.initErr != nil {
		return s.initErr
	}
	if s.baseProfile == nil {
		return fmt.Errorf("local user snapshot is unavailable")
	}
	return nil
}

func (s *Service) DetailedProfile(region renderregion.Value) *drawing.DetailedProfileCardRequest {
	if s == nil || s.baseProfile == nil {
		return nil
	}
	profile := *s.baseProfile
	if normalized := renderregion.WithDefault(region); !normalized.IsZero() {
		profile.Region = strings.ToUpper(normalized.String())
	}
	profile.Mode = common.CloneStringPtr(s.baseProfile.Mode)
	profile.FramePath = common.CloneStringPtr(s.baseProfile.FramePath)
	profile.UserCards = append([]interface{}(nil), s.baseProfile.UserCards...)
	return &profile
}

func (s *Service) ProfileCard(region renderregion.Value) *drawing.ProfileCardRequest {
	detail := s.DetailedProfile(region)
	if detail == nil {
		return nil
	}
	source := detail.Source
	update := detail.UpdateTime
	return &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       common.CloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:       "User Data",
				Source:     &source,
				UpdateTime: &update,
				Mode:       common.CloneStringPtr(detail.Mode),
			},
		},
	}
}

func (s *Service) MusicResults(diff string) map[int]string {
	if s == nil {
		return nil
	}
	diffKey := strings.ToLower(strings.TrimSpace(diff))
	source := s.musicResult[diffKey]
	copied := make(map[int]string, len(source))
	for musicID, status := range source {
		copied[musicID] = status
	}
	return copied
}

func (s *Service) GetMusicResult(musicID int, diff string) string {
	if s == nil {
		return ""
	}
	diffKey := strings.ToLower(strings.TrimSpace(diff))
	if result, ok := s.musicResult[diffKey][musicID]; ok {
		return result
	}
	return ""
}

func (s *Service) ChallengeLive() *ChallengeLiveData {
	if s == nil || s.challenge == nil {
		return nil
	}
	return &ChallengeLiveData{
		Results: append([]ChallengeLiveResult(nil), s.challenge.Results...),
		Stages:  append([]ChallengeLiveStage(nil), s.challenge.Stages...),
		Rewards: append([]ChallengeLiveReward(nil), s.challenge.Rewards...),
	}
}

func (s *Service) RawBytes() ([]byte, error) {
	if s == nil || len(s.rawJSON) == 0 {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	return append([]byte(nil), s.rawJSON...), nil
}

func (s *Service) RawFilePath() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.rawFilePath)
}

func (s *Service) RawData() *RawUserData {
	if s == nil {
		return nil
	}
	return s.rawData
}

func (s *Service) MusicMetaBytes() []byte {
	if s == nil || len(s.musicMetaBytes) == 0 {
		return nil
	}
	return append([]byte(nil), s.musicMetaBytes...)
}

func (s *Service) MusicMetaPath() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.musicMetaPath)
}

