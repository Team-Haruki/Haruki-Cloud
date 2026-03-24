package userdata

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

// NewFromBytes constructs a Service from raw JSON bytes obtained from live API calls,
// rather than from local files. This is the live-data path used in production.
//
//   - suiteJSON:      required; raw bytes from GetSuiteData() (equivalent to user.json)
//   - mysekaiJSON:    optional (nil = skip); raw bytes from GetMySekaiData() (equivalent to mysekai.json)
//   - musicMetaBytes: optional (nil = skip); raw bytes from the music-meta cache
//
// sekaiClient and assetHelper are used to resolve the leader card image path for rendering.
func NewFromBytes(
	sekaiClient *sekaiDB.Client,
	assetHelper *assets.AssetHelper,
	region renderregion.Value,
	suiteJSON []byte,
	mysekaiJSON []byte,
	musicMetaBytes []byte,
) (*Service, error) {
	service := &Service{
		configured:  true,
		musicResult: make(map[string]map[int]string),
	}

	defaultRegion := renderregion.WithDefault(region)

	data := suiteJSON

	if len(mysekaiJSON) > 0 {
		merged, err := mergeMySekaiBytes(data, mysekaiJSON)
		if err != nil {
			return nil, err
		}
		data = merged
	}

	var raw RawUserData
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("userdata: decode suite JSON: %w", err)
	}
	if raw.UserGamedata.UserID == 0 {
		return nil, fmt.Errorf("userdata: suite JSON is missing userId")
	}

	activeDeck := findActiveDeck(raw.UserDecks, raw.UserGamedata.Deck)
	leaderCardID := activeDeck.Leader
	leaderCard := findUserCard(raw.UserCards, leaderCardID)
	leaderImagePath := resolveLeaderImagePath(sekaiClient, assetHelper, defaultRegion, leaderCardID, isAfterTraining(leaderCard))
	if leaderImagePath == "" {
		leaderImagePath = fallbackLeaderImagePath(assetHelper)
	}

	mode := strings.TrimSpace(raw.UserProfile.ProfileImageType)
	service.baseProfile = &drawing.DetailedProfileCardRequest{
		ID:              strconv.FormatInt(raw.UserGamedata.UserID, 10),
		Region:          strings.ToUpper(defaultRegion.String()),
		Nickname:        raw.UserGamedata.Name,
		Source:          "toolbox_live",
		UpdateTime:      raw.Now,
		Mode:            optionalString(mode),
		IsHideUID:       true,
		LeaderImagePath: leaderImagePath,
		HasFrame:        false,
		UserCards:       buildUserCardEntries(activeDeck),
	}
	service.musicResult = buildMusicResultMap(raw.UserMusicStats)
	service.challenge = &ChallengeLiveData{
		Results: convertChallengeResults(raw.UserChallengeLiveSoloResults),
		Stages:  convertChallengeStages(raw.UserChallengeLiveSoloStages),
		Rewards: convertChallengeRewards(raw.UserChallengeLiveSoloHighScoreRewards),
	}
	service.rawData = &raw
	service.rawJSON = data

	if len(musicMetaBytes) > 0 {
		service.musicMetaBytes = meta.InjectOmakase(musicMetaBytes)
	}

	return service, nil
}

// mergeMySekaiBytes merges mysekai JSON bytes into suite JSON bytes in memory,
// following the same merge logic as the file-based path.
func mergeMySekaiBytes(suiteData, mysekaiData []byte) ([]byte, error) {
	var baseMap map[string]interface{}
	if err := json.Unmarshal(suiteData, &baseMap); err != nil {
		return nil, fmt.Errorf("userdata: decode suite JSON for mysekai merge: %w", err)
	}

	var mysekaiMap map[string]interface{}
	if err := json.Unmarshal(mysekaiData, &mysekaiMap); err != nil {
		return nil, fmt.Errorf("userdata: decode mysekai JSON: %w", err)
	}

	if updatedResources, ok := mysekaiMap["updatedResources"].(map[string]interface{}); ok {
		for key, value := range updatedResources {
			baseMap[key] = value
		}
	}
	for key, value := range mysekaiMap {
		if key == "updatedResources" {
			continue
		}
		baseMap[key] = value
	}

	merged, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("userdata: encode merged mysekai: %w", err)
	}
	return merged, nil
}
