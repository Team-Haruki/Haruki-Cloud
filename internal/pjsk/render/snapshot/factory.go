package snapshot

import (
	"bytes"
	"context"
	"fmt"
	json "github.com/bytedance/sonic"
	"slices"
	"strconv"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
)

type BuildInput struct {
	Region         renderregion.Value
	Source         string
	SuiteJSON      []byte
	MySekaiJSON    []byte
	MusicMetaJSON  []byte
	MusicMetaPath  string
	PersistRawFile bool
	RawFilePattern string
}

type HarukiSnapshotFactory interface {
	Build(ctx context.Context, input BuildInput) (Snapshot, error)
}

type DefaultSnapshotFactory struct {
	sekaiClient *sekaiDB.Client
	assetHelper *assets.AssetHelper
}

func NewDefaultSnapshotFactory(sekaiClient *sekaiDB.Client, assetHelper *assets.AssetHelper) *DefaultSnapshotFactory {
	return &DefaultSnapshotFactory{
		sekaiClient: sekaiClient,
		assetHelper: assetHelper,
	}
}

func (f *DefaultSnapshotFactory) Build(ctx context.Context, input BuildInput) (Snapshot, error) {
	if ctx == nil {
		ctx = context.TODO()
	}
	finishBuild := commandtrace.MeasureOperation(ctx, "snapshot.build")
	defer finishBuild()
	if len(bytes.TrimSpace(input.SuiteJSON)) == 0 {
		return nil, fmt.Errorf("snapshot: suite snapshot is empty")
	}

	var (
		data []byte
		err  error
	)
	finishNormalize := commandtrace.MeasureOperation(ctx, "snapshot.normalize")
	if len(bytes.TrimSpace(input.MySekaiJSON)) > 0 {
		data, err = mergeMySekaiData(input.SuiteJSON, input.MySekaiJSON)
		if err != nil {
			finishNormalize()
			return nil, err
		}
	} else {
		data, err = normalizeSnapshotJSON(input.SuiteJSON)
		if err != nil {
			finishNormalize()
			return nil, err
		}
	}
	finishNormalize()

	return f.buildService(ctx, input, data)
}

func (f *DefaultSnapshotFactory) buildService(ctx context.Context, input BuildInput, data []byte) (*Service, error) {
	service := &Service{
		configured:  true,
		musicResult: make(map[string]map[int]string),
	}

	var raw RawUserData
	finishDecode := commandtrace.MeasureOperation(ctx, "snapshot.decode")
	err := json.Unmarshal(data, &raw)
	finishDecode()
	if err != nil {
		return nil, fmt.Errorf("snapshot: decode suite JSON: %w", err)
	}
	if raw.UserGamedata.UserID == 0 {
		return nil, fmt.Errorf("snapshot: suite JSON is missing userId")
	}

	finishModel := commandtrace.MeasureOperation(ctx, "snapshot.model")
	region := renderregion.WithDefault(input.Region)
	activeDeck := FindActiveDeck(raw.UserDecks, raw.UserGamedata.Deck)
	leaderCardID := activeDeck.Leader
	leaderCard := FindUserCard(raw.UserCards, leaderCardID)
	leaderImagePath := resolveLeaderImagePath(ctx, f.sekaiClient, f.assetHelper, region, leaderCardID, leaderCardUsesTrainedArt(leaderCard))
	if leaderImagePath == "" {
		leaderImagePath = fallbackLeaderImagePath(f.assetHelper)
	}

	mode := strings.TrimSpace(raw.UserProfile.ProfileImageType)
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "snapshot"
	}
	service.baseProfile = &drawing.DetailedProfileCardRequest{
		ID:              strconv.FormatInt(raw.UserGamedata.UserID, 10),
		Region:          strings.ToUpper(region.String()),
		Nickname:        raw.UserGamedata.Name,
		Source:          source,
		UpdateTime:      raw.Now,
		Mode:            common.OptionalString(mode),
		IsHideUID:       true,
		LeaderImagePath: leaderImagePath,
		HasFrame:        false,
		UserCards:       buildUserCardEntries(raw.UserCards),
	}
	service.musicResult = resolveMusicResultMap(raw)
	service.challenge = &ChallengeLiveData{
		Results: convertChallengeResults(raw.UserChallengeLiveSoloResults),
		Stages:  convertChallengeStages(raw.UserChallengeLiveSoloStages),
		Rewards: convertChallengeRewards(raw.UserChallengeLiveSoloHighScoreRewards),
	}
	service.rawData = &raw
	service.rawJSON = slices.Clone(data)

	if len(input.MusicMetaJSON) > 0 {
		processed, view, prepareErr := meta.Prepare(input.MusicMetaJSON)
		if prepareErr != nil {
			// Preserve the historical best-effort behavior for optional metadata:
			// the user snapshot remains usable even if this side payload is bad.
			service.musicMetaBytes = slices.Clone(input.MusicMetaJSON)
		} else {
			service.musicMetaBytes = processed
			service.musicMetaView = view
		}
		service.musicMetaPath = strings.TrimSpace(input.MusicMetaPath)
	}
	finishModel()

	if input.PersistRawFile {
		finishPersist := commandtrace.MeasureOperation(ctx, "snapshot.persist")
		pattern := strings.TrimSpace(input.RawFilePattern)
		if pattern == "" {
			pattern = "haruki-pjsk-user-*.json"
		}
		rawFilePath, err := writeNormalizedSnapshotFile(pattern, data)
		if err != nil {
			finishPersist()
			return nil, err
		}
		service.rawFilePath = rawFilePath
		finishPersist()
	}

	return service, nil
}
