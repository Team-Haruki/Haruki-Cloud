package pjsk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	json "github.com/bytedance/sonic"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	botDB "haruki-cloud/database/bot"
	botcommandlog "haruki-cloud/database/bot/commandlog"
	botdailyrequests "haruki-cloud/database/bot/dailyrequests"
	botenttest "haruki-cloud/database/bot/enttest"
	bothourlyrequests "haruki-cloud/database/bot/hourlyrequests"
	botrequestsranking "haruki-cloud/database/bot/requestsranking"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	noiseCrypto "haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/identity"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	commandhandler "haruki-cloud/internal/pjsk/handler"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
	noiseMP "github.com/shamaton/msgpack/v3"
)

const testBotID = "11451419"

type botBindingValidator struct{}

func (botBindingValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	return nil, sekaiapi.ErrUserNotFound
}

type botBindingJPValidator struct{}

func (botBindingJPValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if strings.EqualFold(server, "jp") {
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 1234567890,
				Name:   "JPBoundUser",
			},
		}, nil
	}
	return nil, sekaiapi.ErrUserNotFound
}

type botBindingJPENValidator struct{}

func (botBindingJPENValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	switch {
	case strings.EqualFold(server, "jp") && userID == "13200000000982":
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 13200000000982,
				Name:   "JPBoundUser",
			},
		}, nil
	case strings.EqualFold(server, "en") && userID == "39400000000123":
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 39400000000123,
				Name:   "ENBoundUser",
			},
		}, nil
	default:
		return nil, sekaiapi.ErrUserNotFound
	}
}

type botBindingCNValidator struct{}

func (botBindingCNValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if strings.EqualFold(server, "cn") {
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 2234567890,
				Name:   "CNBoundUser",
			},
		}, nil
	}
	return nil, sekaiapi.ErrUserNotFound
}

type botBindingMultiRegionValidator struct{}

func (botBindingMultiRegionValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	switch {
	case strings.EqualFold(server, "cn") && userID == "74800000000663":
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 74800000000663,
				Name:   "CNBoundUser",
			},
		}, nil
	case strings.EqualFold(server, "jp") && userID == "13200000000982":
		return &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 13200000000982,
				Name:   "JPBoundUser",
			},
		}, nil
	default:
		return nil, sekaiapi.ErrUserNotFound
	}
}

type botTrackerSource struct{}

type botLegacyTrackerSource interface {
	GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error)
	GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error)
	GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error)
	GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error)
	TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error)
	TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error)
	TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error)
	TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error)
}

func setBotTrackerIntegration(controller *rendersk.Controller, tracker botLegacyTrackerSource, events rendersk.EventSource, assetHelper *assets.AssetHelper) {
	controller.SetTrackerIntegration(botCloudV2TrackerSource{botLegacyTrackerSource: tracker}, events, assetHelper)
}

type botCloudV2TrackerSource struct {
	botLegacyTrackerSource
}

func (s botCloudV2TrackerSource) GetCloudSKQuery(server string, eventID int, characterID *int, ranks []int, userID *int64, includeAdjacent, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudRankQueryResponse, error) {
	out := &sekaiapi.CloudRankQueryResponse{}
	if userID != nil && *userID > 0 {
		item, err := s.cloudRankInfoByUser(server, eventID, characterID, *userID)
		if err != nil {
			return nil, err
		}
		if item.Rank <= 0 {
			return nil, sekaiapi.ErrRankingNotFound
		}
		out.Ranks = append(out.Ranks, item)
		return out, nil
	}
	for _, rank := range ranks {
		item, err := s.cloudRankInfoByRank(server, eventID, characterID, rank)
		if err != nil {
			if skipMissing && errors.Is(err, sekaiapi.ErrRankingNotFound) {
				continue
			}
			return nil, err
		}
		out.Ranks = append(out.Ranks, item)
	}
	return out, nil
}

func (s botCloudV2TrackerSource) GetCloudSKCheckRoom(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, userID, true, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	if len(resp.Ranks) == 0 {
		return nil, sekaiapi.ErrRankingNotFound
	}
	return &sekaiapi.CloudCheckRoomResponse{Rank: resp.Ranks[0], Ranks: resp.Ranks}, nil
}

func (s botCloudV2TrackerSource) GetCloudSKLine(server string, eventID int, characterID *int, ranks []int, userID *int64, skipMissing bool, intervalSeconds int64) (*sekaiapi.CloudLineResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, userID, false, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	for i := range resp.Ranks {
		resp.Ranks[i].Name = ""
	}
	return &sekaiapi.CloudLineResponse{Ranks: resp.Ranks}, nil
}

func (s botCloudV2TrackerSource) GetCloudSKSpeed(server string, eventID int, characterID *int, ranks []int, intervalSeconds, unitSeconds int64, skipMissing bool) (*sekaiapi.CloudSpeedResponse, error) {
	resp, err := s.GetCloudSKQuery(server, eventID, characterID, ranks, nil, false, skipMissing, intervalSeconds)
	if err != nil {
		return nil, err
	}
	for i := range resp.Ranks {
		speed := 1000
		resp.Ranks[i].Speed = &speed
	}
	return &sekaiapi.CloudSpeedResponse{Speeds: resp.Ranks, IntervalSeconds: intervalSeconds, UnitSeconds: unitSeconds}, nil
}

func (s botCloudV2TrackerSource) GetCloudSKTrace(server string, eventID int, characterID *int, subjectType string, subject string, limit int) (*sekaiapi.CloudTraceResponse, error) {
	points, userData, err := s.cloudTracePoints(server, eventID, characterID, subjectType, subject)
	if err != nil {
		return nil, err
	}
	out := make([]sekaiapi.CloudRankInfo, 0, len(points))
	name := userData.Name
	for _, point := range points {
		userID := point.UserID
		out = append(out, sekaiapi.CloudRankInfo{Rank: point.Rank, UserID: stringPtr(userID), Name: name, Score: point.Score, Timestamp: point.Timestamp, CharacterID: characterID})
	}
	return &sekaiapi.CloudTraceResponse{Subject: sekaiapi.SubjectTraceMeta{SubjectType: subjectType, Subject: subject}, RankData: out}, nil
}

func (s botCloudV2TrackerSource) GetEventStatus(server string, eventID int) (*sekaiapi.EventStatusResponse, error) {
	source, ok := s.botLegacyTrackerSource.(interface {
		GetEventStatus(string, int) (*sekaiapi.EventStatusResponse, error)
	})
	if !ok {
		return nil, fmt.Errorf("not implemented")
	}
	return source.GetEventStatus(server, eventID)
}

func (s botCloudV2TrackerSource) cloudRankInfoByRank(server string, eventID int, characterID *int, rank int) (sekaiapi.CloudRankInfo, error) {
	if characterID != nil {
		resp, err := s.GetLatestWorldBloomRankingByRank(server, eventID, *characterID, rank)
		if err != nil {
			return sekaiapi.CloudRankInfo{}, err
		}
		return cloudRankInfoFromPoint(resp.RankData.RankDataPoint, resp.UserData, characterID), nil
	}
	resp, err := s.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return sekaiapi.CloudRankInfo{}, err
	}
	return cloudRankInfoFromPoint(resp.RankData, resp.UserData, nil), nil
}

func (s botCloudV2TrackerSource) cloudRankInfoByUser(server string, eventID int, characterID *int, userID int64) (sekaiapi.CloudRankInfo, error) {
	if characterID != nil {
		resp, err := s.GetLatestWorldBloomRankingByUser(server, eventID, *characterID, userID)
		if err != nil {
			return sekaiapi.CloudRankInfo{}, err
		}
		return cloudRankInfoFromPoint(resp.RankData.RankDataPoint, resp.UserData, characterID), nil
	}
	resp, err := s.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return sekaiapi.CloudRankInfo{}, err
	}
	return cloudRankInfoFromPoint(resp.RankData, resp.UserData, nil), nil
}

func (s botCloudV2TrackerSource) cloudTracePoints(server string, eventID int, characterID *int, subjectType string, subject string) ([]sekaiapi.RankDataPoint, sekaiapi.RankingUserData, error) {
	userID, _ := strconv.ParseInt(subject, 10, 64)
	rank, _ := strconv.Atoi(subject)
	if subjectType == "user" {
		if characterID != nil {
			resp, err := s.TraceWorldBloomRankingByUser(server, eventID, *characterID, userID)
			if err != nil {
				return nil, sekaiapi.RankingUserData{}, err
			}
			return flattenBotWorldBloom(resp.RankData), resp.UserData, nil
		}
		resp, err := s.TraceRankingByUser(server, eventID, userID)
		if err != nil {
			return nil, sekaiapi.RankingUserData{}, err
		}
		return resp.RankData, resp.UserData, nil
	}
	if characterID != nil {
		resp, err := s.TraceWorldBloomRankingByRank(server, eventID, *characterID, rank)
		if err != nil {
			return nil, sekaiapi.RankingUserData{}, err
		}
		return flattenBotWorldBloom(resp.RankData), resp.UserData, nil
	}
	resp, err := s.TraceRankingByRank(server, eventID, rank)
	if err != nil {
		return nil, sekaiapi.RankingUserData{}, err
	}
	return resp.RankData, resp.UserData, nil
}

func cloudRankInfoFromPoint(point sekaiapi.RankDataPoint, userData sekaiapi.RankingUserData, characterID *int) sekaiapi.CloudRankInfo {
	userID := point.UserID
	if userID == "" {
		userID = userData.UserID
	}
	return sekaiapi.CloudRankInfo{Rank: point.Rank, UserID: stringPtr(userID), Name: userData.Name, Score: point.Score, Timestamp: point.Timestamp, CharacterID: characterID}
}

func flattenBotWorldBloom(points []sekaiapi.WorldBloomRankDataPoint) []sekaiapi.RankDataPoint {
	out := make([]sekaiapi.RankDataPoint, 0, len(points))
	for _, point := range points {
		out = append(out, point.RankDataPoint)
	}
	return out
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

type botTrackerMissingUserSource struct {
	botTrackerSource
}

type botTrackerStaleSelfSource struct {
	botTrackerSource
	healthy        bool
	rank           int
	currentInRange bool
}

func (botTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	score := 3000000 + rank
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "10001",
			Score:     score,
			Rank:      rank,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "10001",
			Name:   "BotTrackerUser",
		},
	}, nil
}

func (botTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	score := 5000000 + int(userID%1000)
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    "10002",
			Score:     score,
			Rank:      777,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "10002",
			Name:   "BotTrackerUIDUser",
		},
	}, nil
}

func (s botTrackerStaleSelfSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	resp, err := s.botTrackerSource.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return nil, err
	}
	rank := s.rank
	if rank <= 0 {
		rank = 100
	}
	resp.RankData.Rank = rank
	resp.RankData.Timestamp = time.Now().UTC().Add(-6 * time.Minute).Unix()
	if s.currentInRange {
		resp.RankData.UserID = strconv.FormatInt(userID, 10)
		resp.UserData.UserID = strconv.FormatInt(userID, 10)
	}
	return resp, nil
}

func (s botTrackerStaleSelfSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	resp, err := s.botTrackerSource.TraceRankingByUser(server, eventID, userID)
	if err != nil {
		return nil, err
	}
	rank := s.rank
	if rank <= 0 {
		rank = 100
	}
	for i := range resp.RankData {
		resp.RankData[i].Rank = rank
		resp.RankData[i].Timestamp = time.Now().UTC().Add(time.Duration(i-7) * time.Minute).Unix()
	}
	return resp, nil
}

func (s botTrackerStaleSelfSource) GetEventStatus(server string, eventID int) (*sekaiapi.EventStatusResponse, error) {
	if !s.healthy {
		return &sekaiapi.EventStatusResponse{
			Status:     2,
			StatusDesc: "sekai api timeout",
			TimeAgo:    0,
		}, nil
	}
	return &sekaiapi.EventStatusResponse{
		Status:     1,
		StatusDesc: "正常",
		TimeAgo:    0,
	}, nil
}

func (botTrackerMissingUserSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return nil, sekaiapi.ErrRankingNotFound
}

func (botTrackerMissingUserSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, sekaiapi.ErrRankingNotFound
}

func (botTrackerMissingUserSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return nil, sekaiapi.ErrRankingNotFound
}

func (botTrackerMissingUserSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, sekaiapi.ErrRankingNotFound
}

func (botTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	score := 4000000 + rank
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    "20001",
				Score:     score,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			CharacterID: &characterID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "20001",
			Name:   "BotWLTrackerUser",
		},
	}, nil
}

func (botTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	score := 6000000 + int(userID%1000)
	return &sekaiapi.WorldBloomLatestRankingResponse{
		RankData: sekaiapi.WorldBloomRankDataPoint{
			RankDataPoint: sekaiapi.RankDataPoint{
				UserID:    "20002",
				Score:     score,
				Rank:      888,
				Timestamp: 1704067200,
			},
			CharacterID: &characterID,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "20002",
			Name:   "BotWLTrackerUIDUser",
		},
	}, nil
}

func (botTrackerSource) GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error) {
	return &sekaiapi.UserEventData{
		UserID: strconv.FormatInt(userID, 10),
		Name:   "BotTrackerEventUser",
	}, nil
}

func (botTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	earlier := int64(1704067200)
	diff := int64(interval)
	growthRank1 := 1200
	growthRank100 := 4500
	earlierRank1 := 3000001
	earlierRank100 := 3100000
	return []sekaiapi.ScoreGrowthPoint{
		{
			Rank:             1,
			ScoreLatest:      earlierRank1 + growthRank1,
			ScoreEarlier:     &earlierRank1,
			TimestampLatest:  earlier + diff,
			TimestampEarlier: &earlier,
			TimeDiff:         &diff,
			Growth:           &growthRank1,
		},
		{
			Rank:             100,
			ScoreLatest:      earlierRank100 + growthRank100,
			ScoreEarlier:     &earlierRank100,
			TimestampLatest:  earlier + diff,
			TimestampEarlier: &earlier,
			TimeDiff:         &diff,
			Growth:           &growthRank100,
		},
	}, nil
}

func (botTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return botTrackerSource{}.GetRankingScoreGrowth(server, eventID, interval)
}

func (botTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    "10001",
				Score:     3000000 + rank,
				Rank:      rank,
				Timestamp: 1704067200,
			},
			{
				UserID:    "10001",
				Score:     3005000 + rank,
				Rank:      rank,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "10001",
			Name:   "BotTrackerUser",
		},
	}, nil
}

func (botTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    "20001",
					Score:     4000000 + rank,
					Rank:      rank,
					Timestamp: 1704067200,
				},
				CharacterID: &characterID,
			},
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    "20001",
					Score:     4005000 + rank,
					Rank:      rank,
					Timestamp: 1704070800,
				},
				CharacterID: &characterID,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: "20001",
			Name:   "BotWLTrackerUser",
		},
	}, nil
}

func (botTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5000000 + int(userID%1000),
				Rank:      777,
				Timestamp: 1704067200,
			},
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5005000 + int(userID%1000),
				Rank:      777,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotTrackerUIDUser",
		},
	}, nil
}

func (botTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return &sekaiapi.WorldBloomTraceRankingResponse{
		RankData: []sekaiapi.WorldBloomRankDataPoint{
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    strconv.FormatInt(userID, 10),
					Score:     6000000 + int(userID%1000),
					Rank:      888,
					Timestamp: 1704067200,
				},
				CharacterID: &characterID,
			},
			{
				RankDataPoint: sekaiapi.RankDataPoint{
					UserID:    strconv.FormatInt(userID, 10),
					Score:     6005000 + int(userID%1000),
					Rank:      888,
					Timestamp: 1704070800,
				},
				CharacterID: &characterID,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotWLTrackerUIDUser",
		},
	}, nil
}

type botCSBTrackerSource struct {
	botTrackerSource
}

func (botCSBTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return &sekaiapi.LatestRankingResponse{
		RankData: sekaiapi.RankDataPoint{
			UserID:    strconv.FormatInt(userID, 10),
			Score:     5_000_001,
			Rank:      1,
			Timestamp: 1704067200,
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotCSBUser",
		},
	}, nil
}

func (botCSBTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return &sekaiapi.TraceRankingResponse{
		RankData: []sekaiapi.RankDataPoint{
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5_000_001,
				Rank:      1,
				Timestamp: 1704067200,
			},
			{
				UserID:    strconv.FormatInt(userID, 10),
				Score:     5_005_001,
				Rank:      1,
				Timestamp: 1704070800,
			},
		},
		UserData: sekaiapi.RankingUserData{
			UserID: strconv.FormatInt(userID, 10),
			Name:   "BotCSBUser",
		},
	}, nil
}

// testBotApp registers bot routes on a fresh Fiber instance.
func testBotApp(t *testing.T, drawingURL string) *fiber.App {
	t.Helper()
	return testBotAppWithDependencies(t, drawingURL, nil, nil)
}

func testBotAppWithBindings(t *testing.T, drawingURL string, bindingService *accountdata.BindingService) *fiber.App {
	t.Helper()
	return testBotAppWithDependencies(t, drawingURL, bindingService, nil)
}

func testBotAppWithDependencies(t *testing.T, drawingURL string, bindingService *accountdata.BindingService, botDBClient *botDB.Client) *fiber.App {
	t.Helper()
	var client *drawing.HarukiDrawingClient
	if drawingURL != "" {
		client = drawing.NewHarukiDrawingClient(drawingURL)
	}
	app := fiber.New()
	runtime := testRenderApp(t, client)
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, botDBClient, nil)
	return app
}

func newBotCommandTestClient(t *testing.T, name string) *botDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:bot_pjsk_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano())
	return botenttest.Open(t, "sqlite3", dsn)
}

func testBindingService(t *testing.T) *accountdata.BindingService {
	return testBindingServiceWithValidator(t, botBindingValidator{})
}

func testBindingServiceWithValidator(t *testing.T, validator accountdata.ProfileValidator) *accountdata.BindingService {
	t.Helper()
	pjskClient := pjskenttest.Open(t, "sqlite3", "file:bot_api_bind_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", "file:bot_api_users_test?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = usersClient.Close() })
	return accountdata.NewBindingService(
		pjskClient,
		identity.NewResolver(usersClient),
		validator,
	)
}

// botPJSKPath returns the full URL for a PJSK bot endpoint.
func botPJSKPath(path string) string {
	return "/api/v2/bot/" + testBotID + "/pjsk/" + path
}

func newBotPOSTRequest(path string, req BotCommandRequest) *http.Request {
	body, _ := json.Marshal(req)
	r, _ := http.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Host = "localhost"
	r.Header.Set("Content-Type", "application/json")
	return r
}

func decodeSuccessMessage(t *testing.T, body []byte) onebot11.Message {
	t.Helper()
	var envelope renderEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, body)
	}
	var message onebot11.Message
	if err := json.Unmarshal(envelope.Data, &message); err != nil {
		t.Fatalf("decode onebot message: %v raw=%s", err, envelope.Data)
	}
	return message
}

func assertSingleImageMessage(t *testing.T, body []byte) {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 1 || message[0].Type != "image" {
		t.Fatalf("expected single image message, got %+v", message)
	}
	data, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected image segment data: %#v", message[0].Data)
	}
	file, _ := data["file"].(string)
	if !strings.HasPrefix(file, "https://image-cache.test/pjsk/") {
		t.Fatalf("unexpected image url: %q", file)
	}
}

func assertTextAndImageMessage(t *testing.T, body []byte, wantText string) {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 2 || message[0].Type != "text" || message[1].Type != "image" {
		t.Fatalf("expected text + image message, got %+v", message)
	}
	textData, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected text segment data: %#v", message[0].Data)
	}
	if text, _ := textData["text"].(string); text != wantText {
		t.Fatalf("expected text %q, got %q", wantText, text)
	}
	imageData, ok := message[1].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected image segment data: %#v", message[1].Data)
	}
	file, _ := imageData["file"].(string)
	if !strings.HasPrefix(file, "https://image-cache.test/pjsk/") {
		t.Fatalf("unexpected image url: %q", file)
	}
}

func assertSingleTextMessage(t *testing.T, body []byte, want string) {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 1 || message[0].Type != "text" {
		t.Fatalf("expected single text message, got %+v", message)
	}
	data, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected text segment data: %#v", message[0].Data)
	}
	text, _ := data["text"].(string)
	if text != want {
		t.Fatalf("expected text %q, got %q", want, text)
	}
}

// ── Endpoint tests ──────────────────────────────────────────────────────────

func TestBotEndpointGetReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointGetReturnsTextJSON(t *testing.T) {
	app := testBotAppWithBindings(t, "", testBindingService(t))

	req := newBotPOSTRequest(botPJSKPath("profile/bind/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", MatchedCommand: "/绑定列表",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/绑定列表"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	assertSingleTextMessage(t, body, "你还没有绑定任何PJSK账号")
}

func TestBotEndpointRegionPrefixedBindListFiltersBindings(t *testing.T) {
	bindings := testBindingServiceWithValidator(t, botBindingMultiRegionValidator{})
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "74800000000663"); err != nil {
		t.Fatalf("bind cn: %v", err)
	}
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/bind/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "cn", MatchedCommand: "/绑定列表",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cn绑定列表"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	assertSingleTextMessage(t, body, "已绑定CN服账号列表（u序号按该区服编号）:\nu1 [CN] 748********663 (全局默认 / CN服默认)")
}

func TestBotEndpointBindListFiltersTransportRegionAfterClientStripsPrefix(t *testing.T) {
	bindings := testBindingServiceWithValidator(t, botBindingMultiRegionValidator{})
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "74800000000663"); err != nil {
		t.Fatalf("bind cn: %v", err)
	}
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/bind/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "cn", MatchedCommand: "/绑定列表",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/绑定列表"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	assertSingleTextMessage(t, body, "已绑定CN服账号列表（u序号按该区服编号）:\nu1 [CN] 748********663 (全局默认 / CN服默认)")
}

func TestBotEndpointRegionPrefixedQueryUIDUsesRegionBinding(t *testing.T) {
	bindings := testBindingServiceWithValidator(t, botBindingMultiRegionValidator{})
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "74800000000663"); err != nil {
		t.Fatalf("bind cn: %v", err)
	}
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/uid"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查uid",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/jp查uid"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	assertSingleTextMessage(t, body, "13200000000982")
}

func TestBotEndpointQueryUIDUsesTransportRegionAfterClientStripsPrefix(t *testing.T) {
	bindings := testBindingServiceWithValidator(t, botBindingMultiRegionValidator{})
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "74800000000663"); err != nil {
		t.Fatalf("bind cn: %v", err)
	}
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/uid"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查uid",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查uid"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	assertSingleTextMessage(t, body, "13200000000982")
}

func TestBotEndpointRegionPrefixedHideIDSyncsProfileSettingsParams(t *testing.T) {
	ctx := context.Background()
	bindings := testBindingServiceWithValidator(t, botBindingJPENValidator{})
	if _, err := bindings.Bind(ctx, "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := bindings.Bind(ctx, "qq", "12345", "39400000000123"); err != nil {
		t.Fatalf("bind en: %v", err)
	}
	if _, err := bindings.SetBindingVisible(ctx, "qq", "12345", "jp", true); err != nil {
		t.Fatalf("show jp id: %v", err)
	}
	if _, err := bindings.SetBindingVisible(ctx, "qq", "12345", "en", true); err != nil {
		t.Fatalf("show en id: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/visibility/hide"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/en隐藏ID",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/隐藏ID"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessage(t, body, "已隐藏 [EN] 394********123 的ID信息")

	items, err := bindings.List(ctx, "qq", "12345")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	for _, item := range items {
		switch item.Server {
		case "jp":
			if !item.Visible {
				t.Fatalf("jp visibility was changed by /en隐藏ID: %+v", item)
			}
		case "en":
			if item.Visible {
				t.Fatalf("en visibility was not hidden: %+v", item)
			}
		}
	}
}

func TestBotEndpointTransportRegionShowSuiteSyncsProfileSettingsParams(t *testing.T) {
	ctx := context.Background()
	bindings := testBindingServiceWithValidator(t, botBindingJPENValidator{})
	if _, err := bindings.Bind(ctx, "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := bindings.Bind(ctx, "qq", "12345", "39400000000123"); err != nil {
		t.Fatalf("bind en: %v", err)
	}
	if _, err := bindings.SetBindingSuiteVisible(ctx, "qq", "12345", "jp", false); err != nil {
		t.Fatalf("hide jp suite: %v", err)
	}
	if _, err := bindings.SetBindingSuiteVisible(ctx, "qq", "12345", "en", false); err != nil {
		t.Fatalf("hide en suite: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/suite/show"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "en", MatchedCommand: "/展示抓包",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/展示抓包"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessage(t, body, "已展示 [EN] 394********123 的抓包信息")

	items, err := bindings.List(ctx, "qq", "12345")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	for _, item := range items {
		switch item.Server {
		case "jp":
			if item.SuiteVisible {
				t.Fatalf("jp suite visibility was changed by EN request: %+v", item)
			}
		case "en":
			if !item.SuiteVisible {
				t.Fatalf("en suite visibility was not shown: %+v", item)
			}
		}
	}
}

func TestBotEndpointRegionPrefixedHideSuiteSyncsProfileSettingsParams(t *testing.T) {
	ctx := context.Background()
	bindings := testBindingServiceWithValidator(t, botBindingJPENValidator{})
	if _, err := bindings.Bind(ctx, "qq", "12345", "13200000000982"); err != nil {
		t.Fatalf("bind jp: %v", err)
	}
	if _, err := bindings.Bind(ctx, "qq", "12345", "39400000000123"); err != nil {
		t.Fatalf("bind en: %v", err)
	}
	app := testBotAppWithBindings(t, "", bindings)

	req := newBotPOSTRequest(botPJSKPath("profile/suite/hide"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/en隐藏抓包",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/隐藏抓包"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessage(t, body, "已隐藏 [EN] 394********123 的抓包信息")

	items, err := bindings.List(ctx, "qq", "12345")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	for _, item := range items {
		switch item.Server {
		case "jp":
			if !item.SuiteVisible {
				t.Fatalf("jp suite visibility was changed by /en隐藏抓包: %+v", item)
			}
		case "en":
			if item.SuiteVisible {
				t.Fatalf("en suite visibility was not hidden: %+v", item)
			}
		}
	}
}

func TestBotEndpointSuppressesParamEchoByDefault(t *testing.T) {
	app := testBotApp(t, "")
	secretParam := "super-secret-param"

	req := newBotPOSTRequest(botPJSKPath("event"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查活动",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查活动 " + secretParam}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	text := singleTextMessageText(t, body)
	if strings.Contains(text, secretParam) {
		t.Fatalf("expected response to redact param %q, got %q", secretParam, text)
	}
	if text != "活动查询参数格式不正确。查看完整用法请发送：/查活动 -help" {
		t.Fatalf("expected redacted parse error with help text, got %q", text)
	}
}

func TestBotEndpointStillRedactsParamEchoWhenEnabled(t *testing.T) {
	app := testBotApp(t, "")
	secretParam := "super-secret-param"

	req := newBotPOSTRequest(botPJSKPath("event"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查活动",
		Message:         onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查活动 " + secretParam}}},
		EnableParamEcho: true,
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	text := singleTextMessageText(t, body)
	if strings.Contains(text, secretParam) {
		t.Fatalf("expected response to redact param %q, got %q", secretParam, text)
	}
	if text != "活动查询参数格式不正确。查看完整用法请发送：/查活动 -help" {
		t.Fatalf("expected redacted parse error with help text, got %q", text)
	}
}

func TestBotEndpointRecordsDistributedStatisticsAndCommandLog(t *testing.T) {
	ctx := context.Background()
	botClient := newBotCommandTestClient(t, "telemetry")
	t.Cleanup(func() { _ = botClient.Close() })
	app := testBotAppWithDependencies(t, "", testBindingService(t), botClient)

	req := newBotPOSTRequest(botPJSKPath("profile/bind/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", PlatformGroupID: "67890",
		Server: "jp", MatchedCommand: "/绑定列表",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/绑定列表"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	rankingRow, err := botClient.RequestsRanking.Query().Where(botrequestsranking.BotIDEQ(11451419)).Only(ctx)
	if err != nil {
		t.Fatalf("load requests ranking: %v", err)
	}
	if rankingRow.Counts != 1 {
		t.Fatalf("unexpected requests ranking counts: got=%d want=1", rankingRow.Counts)
	}

	hourlyRows, err := botClient.HourlyRequests.Query().Where(bothourlyrequests.CountEQ(1)).All(ctx)
	if err != nil {
		t.Fatalf("load hourly requests: %v", err)
	}
	if len(hourlyRows) != 1 {
		t.Fatalf("expected 1 hourly row, got %+v", hourlyRows)
	}

	dailyRows, err := botClient.DailyRequests.Query().Where(botdailyrequests.CountEQ(1)).All(ctx)
	if err != nil {
		t.Fatalf("load daily requests: %v", err)
	}
	if len(dailyRows) != 1 {
		t.Fatalf("expected 1 daily row, got %+v", dailyRows)
	}

	logRow, err := botClient.CommandLog.Query().Only(ctx)
	if err != nil {
		t.Fatalf("load command log: %v", err)
	}
	if logRow.Platform != "qq" || logRow.Pid != testBotID || logRow.Gid != "67890" || logRow.UID != "12345" || logRow.Command != "/绑定列表" {
		t.Fatalf("unexpected command log row: %+v", logRow)
	}
	if logRow.CreatedAt.IsZero() {
		t.Fatalf("expected command log created_at to be set, got %+v", logRow)
	}

	count, err := botClient.CommandLog.Query().
		Where(
			botcommandlog.PlatformEQ("qq"),
			botcommandlog.PidEQ(testBotID),
			botcommandlog.GidEQ("67890"),
			botcommandlog.UIDEQ("12345"),
			botcommandlog.CommandEQ("/绑定列表"),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count command logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 matching command log row, got %d", count)
	}
}

func TestResolveBotCommandFallsBackToMessageMatchForCompactTimeZoneCommand(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/pjsktzHKT"}},
	}, "profile/timezone", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/pjsktzHKT",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetTimeZone {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !strings.EqualFold(params.TimeZone, "HKT") {
		t.Fatalf("unexpected timezone param: %q", params.TimeZone)
	}
	if resolved.RequesterPlatform != "qq" || resolved.RequesterUserID != "12345" {
		t.Fatalf("unexpected requester info: platform=%q user=%q", resolved.RequesterPlatform, resolved.RequesterUserID)
	}
	if resolved.RequesterBotID != testBotID {
		t.Fatalf("unexpected requester bot id: %q", resolved.RequesterBotID)
	}
}

func TestBotRouteEnabledDisablesCostumeRoutes(t *testing.T) {
	for _, path := range []string{"costume/detail", "costume/list", "costume/combo"} {
		if botRouteEnabled(path, false) {
			t.Fatalf("3D disabled instance must not expose %s", path)
		}
		if !botRouteEnabled(path, true) {
			t.Fatalf("3D enabled instance must expose %s", path)
		}
	}
	if !botRouteEnabled("event/detail", false) {
		t.Fatal("3D disabled instance must retain unrelated routes")
	}
}

func TestResolveBotCommandCorrectsShortMatchedCommandToArrestDifficulty(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/逮捕难度 master关闭"}},
	}, "arrest", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/逮捕",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetArrestDiff {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.DifficultyToggles) != 1 {
		t.Fatalf("unexpected toggle count: %d", len(params.DifficultyToggles))
	}
	if params.DifficultyToggles[0].Difficulty != "master" || params.DifficultyToggles[0].Enabled {
		t.Fatalf("unexpected toggle: %+v", params.DifficultyToggles[0])
	}
}

func TestResolveBotCommandCorrectsEventsMatchedAcrossMessageSeparator(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/event saki"}},
	}, "event/list", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/events",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != "event-list" {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}

	var params map[string]any
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if got, ok := params["character_id"].(float64); !ok || int(got) != 2 {
		t.Fatalf("unexpected character_id: %#v", params["character_id"])
	}
}

func TestResolveBotCommandCorrectsCardsMatchedAcrossMessageSeparator(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	resolved, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/card saki"}},
	}, "card/list", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/cards",
	}, testBotID)
	if err != nil {
		t.Fatalf("resolveBotCommand() error = %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != "card-image" {
		t.Fatalf("unexpected mode: %s", resolved.Mode)
	}
	if resolved.Query != "saki" {
		t.Fatalf("unexpected query: %q", resolved.Query)
	}
}

func TestResolveBotCommandRejectsUnrelatedMatchedCommandAcrossEndpoint(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()

	_, err := resolveBotCommand(context.Background(), onebot11.Message{
		{Type: "text", Data: onebot11.TextData{Text: "/card saki"}},
	}, "event/list", BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "12345",
		MatchedCommand: "/events",
	}, testBotID)
	if err == nil {
		t.Fatal("expected resolveBotCommand() error")
	}
	var validationErr *botValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected botValidationError, got %T: %v", err, err)
	}
}

func TestBotEndpointGetWithGroupHeadersReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGGROUP"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", PlatformGroupID: "67890",
		Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	assertSingleImageMessage(t, respBody)
}

func TestBotEndpointPlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	assertSingleImageMessage(t, respBody)
}

func TestBotEndpointOneBotMessageArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGSEG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	req := newBotPOSTRequest(botPJSKPath("card/detail"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/查卡",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/查卡 "}},
			{Type: "text", Data: onebot11.TextData{Text: "1001"}},
		},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	assertSingleImageMessage(t, respBody)
}

func TestBotEndpointSKQueryUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		if strings.TrimSpace(req.Ranks[0].Name) == "" {
			t.Fatalf("expected rank 1 name to be present, got %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKTRACKERPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQuerySupportsRegionPrefixedCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Region != "cn" {
			t.Fatalf("unexpected region: %s", req.Region)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCNPING"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cnsk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cnsk event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryAcceptsBaseMatchedCommandForRegionPrefixedInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Region != "cn" {
			t.Fatalf("unexpected region: %s", req.Region)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKMATCHEDBASE"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cnsk event101 100"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointMysekaiOverviewAcceptsLegacyResourceEndpoint(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("mysekai/resource"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/msa",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/msam"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "没有找到有效的 mysekai 数据")
}

func TestBotEndpointMysekaiTalkListAcceptsMSBCommand(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("mysekai/talk-list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/msb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/msb"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "烤森服务未就绪")
}

func TestBotEndpointMysekaiTalkListAcceptsLegacyBlueprintEndpoint(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	runtime.MySekai = rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true})
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("mysekai/blueprint"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/msb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/msb"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "烤森服务未就绪")
}

func TestBotEndpointSKQueryTreatsRequestServerAsExplicitRegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Region != "en" {
			t.Fatalf("unexpected region: %s", req.Region)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSERVEREN"))
	}))
	defer srv.Close()

	bindings := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindings.Bind(context.Background(), "qq", "12345", "12345678901234"); err != nil {
		t.Fatalf("bind jp account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.Bindings = bindings
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "en", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 100"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKLineUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SklRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		if strings.TrimSpace(req.Ranks[0].Name) != "" || strings.TrimSpace(req.Ranks[1].Name) != "" {
			t.Fatalf("expected line request to omit names, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLINEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/line"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skl event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryUsesTrackerUIDPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 {
			t.Fatalf("unexpected ranks len: %d", len(req.Ranks))
		}
		if req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected rank: %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKUIDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 1234567890"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryRankOneShowsPlayerName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 1 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		if strings.TrimSpace(req.Ranks[0].Name) == "" {
			t.Fatalf("expected rank name to be present, got %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKRANK1PNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKLineUsesTrackerUIDPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SklRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 {
			t.Fatalf("unexpected ranks len: %d", len(req.Ranks))
		}
		if req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected rank: %+v", req.Ranks[0])
		}
		if strings.TrimSpace(req.Ranks[0].Name) != "" {
			t.Fatalf("expected line request to omit names, got %+v", req.Ranks[0])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLUIDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/line"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skl event101 1234567890"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKLineDefaultsToExpandedRanksAndOmitsNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/line" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SklRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 34 {
			t.Fatalf("unexpected default line count: %d", len(req.Ranks))
		}
		for _, rank := range req.Ranks {
			if strings.TrimSpace(rank.Name) != "" {
				t.Fatalf("expected line request names to be omitted, got %+v", rank)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKLDEFAULTPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/line"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skl event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryUsesTrackerAtBindingPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKATPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "67890", "1234567890"); err != nil {
		t.Fatalf("bind test account: %v", err)
	}
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "67890", "jp", true); err != nil {
		t.Fatalf("set binding visible: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk event101 "}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
		},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryHandlesInlineCQAtInTextSegment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 777 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKINLINECQAT"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "67890", "1234567890"); err != nil {
		t.Fatalf("bind test account: %v", err)
	}
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "67890", "jp", true); err != nil {
		t.Fatalf("set binding visible: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk event101 [CQ:at,qq=67890]"}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
		},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryReturnsTextWhenTrackerQueryFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called on tracker validation error")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	// Intentionally keep events=nil so /sk without event id triggers tracker-side validation error.
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "当前没有可推断的活动，请指定活动ID")
}

func TestBotEndpointSKQueryDefaultsToSelfBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 777 {
			t.Fatalf("expected self-bound rank payload, got %+v", req.Ranks)
		}
		if req.PrevRanks == nil || req.PrevRanks.Rank != 500 {
			t.Fatalf("unexpected prev ranks: %+v", req.PrevRanks)
		}
		if req.NextRanks == nil || req.NextRanks.Rank != 1000 {
			t.Fatalf("unexpected next ranks: %+v", req.NextRanks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSELFPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryWarnsWhenSelfRecordIsStaleAndTrackerIsHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSTALEPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerStaleSelfSource{healthy: true, rank: 101}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertTextAndImageMessage(t, body, rendersk.StaleSelfRecordWarning)
}

func TestBotEndpointSKQueryDoesNotWarnWhenTrackerStatusIsUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSTALEPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerStaleSelfSource{healthy: false, rank: 101}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKQueryReturnsFriendlyMessageWhenSelfRankingIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when self ranking is missing")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerMissingUserSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "当前JP服活动没有找到你的排行榜数据")
}

func TestBotEndpointSKCheckRoomReturnsFriendlyMessageWhenSelfRankingIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when self ranking is missing")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerMissingUserSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cf",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cf event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "当前JP服活动没有找到你的排行榜数据")
}

func TestBotEndpointSKCSBReturnsFriendlyMessageWhenSelfRankingIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when self ranking is missing")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerMissingUserSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/csb"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/csb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/csb event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "当前JP服活动没有找到你的排行榜数据")
}

func TestBotEndpointSKCSBDoesNotWarnWhenCurrentSelfRecordStillInRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/csb" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CSBRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if len(req.Ranks) == 0 || req.Ranks[0].Rank != 100 {
			t.Fatalf("expected stale self csb payload, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCSBSTALEPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerStaleSelfSource{healthy: true, currentInRange: true}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/csb"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/csb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/csb event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKPlayerTraceReturnsFriendlyMessageWhenSelfRankingIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when self ranking is missing")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerMissingUserSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/player-trace"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/ptr",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/ptr event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "当前JP服活动没有找到你的排行榜数据")
}

func TestBotEndpointSKPlayerTraceReturnsFriendlyMessageWhenDrawingDataIsInsufficient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/player-trace" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"single positional indexer is out-of-bounds"}`))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/player-trace"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/ptr",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/ptr event101 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "玩家轨迹数据不足，暂时无法渲染")
}

func TestBotEndpointSKQueryRegionPrefixedCommandDoesNotFallbackToTransportServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when tw binding is missing")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingCNValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "2234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerMissingUserSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "cn", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/twsk"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "未找到绑定的游戏账号")
	if strings.Contains(string(body), "当前CN服活动没有找到你的排行榜数据") {
		t.Fatalf("unexpected fallback to cn binding: %s", body)
	}
}

func TestBotEndpointSKCSBRegionPrefixedCommandDoesNotFallbackToTransportServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when tw binding is missing")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingCNValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "2234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerMissingUserSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/csb"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "cn", MatchedCommand: "/csb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/twcsb"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "未找到绑定的游戏账号")
	if strings.Contains(string(body), "当前CN服活动没有找到你的排行榜数据") {
		t.Fatalf("unexpected fallback to cn binding: %s", body)
	}
}

func TestBotEndpointSKQueryReturnsTextWhenTargetUserIsHidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("drawing endpoint should not be called when target user is hidden")
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "67890", "1234567890"); err != nil {
		t.Fatalf("bind test account: %v", err)
	}
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "67890", "jp", false); err != nil {
		t.Fatalf("set binding invisible: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk [CQ:at,qq=67890]"}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
		},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "已隐藏个人信息")
}

func TestBotEndpointSKQueryAllowsHiddenSelfBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/query" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SKRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.ID != 101 {
			t.Fatalf("unexpected event id: %d", req.ID)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 777 {
			t.Fatalf("expected self-bound rank payload, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKHIDDENSELFPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}
	if _, err := bindingService.SetBindingVisible(context.Background(), "qq", "12345", "jp", false); err != nil {
		t.Fatalf("set requester binding invisible: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/query"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sk",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sk event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKSpeedUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/speed" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.SpeedRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.EventID != 101 {
			t.Fatalf("unexpected event id: %d", req.EventID)
		}
		if req.RequestType != "时" {
			t.Fatalf("unexpected request type: %s", req.RequestType)
		}
		if req.Period <= 0 {
			t.Fatalf("unexpected period: %d", req.Period)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("expected non-empty ranks in speed request")
		}
		foundRank100 := false
		for _, r := range req.Ranks {
			if r.Rank == 100 {
				foundRank100 = true
				break
			}
		}
		if !foundRank100 {
			t.Fatalf("expected rank 100 in speed request, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKSPEEDPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/speed"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/sks",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/sks event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func assertSingleTextMessageContains(t *testing.T, body []byte, wantPart string) {
	t.Helper()
	text := singleTextMessageText(t, body)
	if !strings.Contains(text, wantPart) {
		t.Fatalf("expected text to contain %q, got %q", wantPart, text)
	}
}

func singleTextMessageText(t *testing.T, body []byte) string {
	t.Helper()
	message := decodeSuccessMessage(t, body)
	if len(message) != 1 || message[0].Type != "text" {
		t.Fatalf("expected single text message, got %+v", message)
	}
	data, ok := message[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected text segment data: %#v", message[0].Data)
	}
	text, _ := data["text"].(string)
	return text
}

func TestBotEndpointSKCheckRoomUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Eid != 101 {
			t.Fatalf("unexpected event id: %d", req.Eid)
		}
		if len(req.Ranks) != 2 || req.Ranks[0].Rank != 1 || req.Ranks[1].Rank != 100 {
			t.Fatalf("unexpected ranks: %+v", req.Ranks)
		}
		if strings.TrimSpace(req.Ranks[0].Name) == "" || strings.TrimSpace(req.Ranks[1].Name) == "" {
			t.Fatalf("expected rank names to be present, got %+v", req.Ranks)
		}
		if req.AggregateAt <= 0 {
			t.Fatalf("aggregate_at should be set")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cf",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cf event101 100 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKCheckRoomDefaultsToSelfBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Eid != 101 {
			t.Fatalf("unexpected event id: %d", req.Eid)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 1 {
			t.Fatalf("expected self-bound check-room payload, got %+v", req.Ranks)
		}
		if req.PrevRank != nil {
			t.Fatalf("unexpected prev rank: %+v", req.PrevRank)
		}
		if req.NextRank == nil || req.NextRank.Rank != 2 {
			t.Fatalf("unexpected next rank: %+v", req.NextRank)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKSELFPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botCSBTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cf",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cf event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKCheckRoomDoesNotWarnWhenCurrentSelfRecordStillInRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if len(req.Ranks) != 1 || req.Ranks[0].Rank != 100 {
			t.Fatalf("expected stale self check-room payload, got %+v", req.Ranks)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKSTALEPNG"))
	}))
	defer srv.Close()

	bindingService := testBindingServiceWithValidator(t, botBindingJPValidator{})
	if _, err := bindingService.Bind(context.Background(), "qq", "12345", "1234567890"); err != nil {
		t.Fatalf("bind requester account: %v", err)
	}

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerStaleSelfSource{healthy: true, currentInRange: true}, nil, assets.NewAssetHelper("", nil))
	runtime.Bindings = bindingService
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cf",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cf event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKCheckRoomLiteUsesFixedRanks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/check-room" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CFRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		wantRanks := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 20, 30, 40, 50, 100}
		if len(req.Ranks) != len(wantRanks) {
			t.Fatalf("unexpected /cfl rank count: %d", len(req.Ranks))
		}
		for i, want := range wantRanks {
			if req.Ranks[i].Rank != want {
				t.Fatalf("unexpected /cfl ranks: %+v", req.Ranks)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCHECKLITEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/cfl",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/cfl event101"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKCheckRoomLegacyCSBCompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/csb" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.CSBRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.Eid != 101 {
			t.Fatalf("unexpected event id: %d", req.Eid)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("expected csb trace payload, got empty ranks")
		}
		if req.Ranks[len(req.Ranks)-1].Rank != 1 {
			t.Fatalf("unexpected latest rank payload: %+v", req.Ranks[len(req.Ranks)-1])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKCSBPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botCSBTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/check-room"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/csb",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/csb event101 1"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKRankTraceUsesTrackerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/rank-trace" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.RankTraceRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.EventID != 101 {
			t.Fatalf("unexpected event id: %d", req.EventID)
		}
		if req.TargetRank != 100 {
			t.Fatalf("unexpected target rank: %d", req.TargetRank)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("rank trace points should not be empty")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKTRACEPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/rank-trace"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/skt",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/skt event101 100"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointSKPlayerTraceSupportsTwoRanks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/sk/player-trace" {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		var req drawing.PlayerTraceRequest
		if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode drawing request: %v", err)
		}
		if req.EventID != 101 {
			t.Fatalf("unexpected event id: %d", req.EventID)
		}
		if len(req.Ranks) == 0 {
			t.Fatalf("first rank trace should not be empty")
		}
		if len(req.Ranks2) == 0 {
			t.Fatalf("second rank trace should not be empty")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SKPTRPNG"))
	}))
	defer srv.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(srv.URL))
	runtime.SK = rendersk.NewController(runtime.Drawing)
	setBotTrackerIntegration(runtime.SK, botTrackerSource{}, nil, assets.NewAssetHelper("", nil))
	RegisterPJSKBotRoutes(app, runtime, nil, nil, nil)

	req := newBotPOSTRequest(botPJSKPath("sk/player-trace"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/ptr",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/ptr event101 1 2"}}},
	})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleImageMessage(t, body)
}

func TestBotEndpointProfileTimeZoneCompatReroutesLegacyProfilePath(t *testing.T) {
	app := testBotAppWithBindings(t, "", testBindingService(t))

	req := newBotPOSTRequest(botPJSKPath("profile/check-data"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/pjsktz",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/pjsktz HKT"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessage(t, body, "已设置PJSK时区为 Asia/Hong_Kong")
}

func TestBotEndpointWrongCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	// /卡面 resolves to card/image, but we send it to card/list
	req := newBotPOSTRequest(botPJSKPath("card/list"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/卡面",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/卡面 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, respBody)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, respBody)
	}
	if envelope.Message != "当前接口不允许使用该 matched_command" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestBotEndpointEmptyCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotPOSTRequest(botPJSKPath("card/image"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/卡面",
		// Message is empty
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointUnknownMatchedCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotPOSTRequest(botPJSKPath("card/image"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp", MatchedCommand: "/不存在的命令",
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/卡面 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointMissingMatchedCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	req := newBotPOSTRequest(botPJSKPath("card/image"), BotCommandRequest{
		Platform: "qq", PlatformUserID: "12345", Server: "jp",
		// MatchedCommand is empty
		Message: onebot11.Message{{Type: "text", Data: onebot11.TextData{Text: "/卡面 1001"}}},
	})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotEndpointGetRejected(t *testing.T) {
	app := testBotApp(t, "")

	req, _ := http.NewRequest(http.MethodGet, botPJSKPath("card/detail"), nil)
	req.Host = "localhost"

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404/405 for GET, got %d", resp.StatusCode)
	}
}

func TestBotManifestEndpoint(t *testing.T) {
	app := testBotApp(t, "")

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	req.Host = "localhost"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	// With nil botDBClient, the endpoint returns 501 Not Implemented.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no DB client), got %d body=%s", resp.StatusCode, respBody)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode manifest: %v raw=%s", err, respBody)
	}
	if !strings.Contains(envelope.Message, "指令清单不可用") {
		t.Fatalf("expected unavailable manifest message, got: %s", envelope.Message)
	}
}

func TestBotManifestEndpointIncludesClientPolicyScopes(t *testing.T) {
	botClient := newBotCommandTestClient(t, "manifest_client_policy")
	t.Cleanup(func() { _ = botClient.Close() })
	app := testBotAppWithDependencies(t, "", nil, botClient)

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	req.Host = "localhost"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}

	var envelope struct {
		Data ManifestResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode manifest: %v raw=%s", err, respBody)
	}
	want := map[string]string{
		"profile/custom-profile-card": "custom_profile",
		"mysekai/birthday-monitor":    "birthday_monitor",
	}
	for _, entry := range envelope.Data.Entries {
		scope, ok := want[entry.CommandPath]
		if !ok {
			continue
		}
		if entry.CommandModule != "pjsk" {
			t.Fatalf("%s module = %q", entry.CommandPath, entry.CommandModule)
		}
		if entry.ClientPolicyScope != scope {
			t.Fatalf("%s client policy scope = %q", entry.CommandPath, entry.ClientPolicyScope)
		}
		delete(want, entry.CommandPath)
	}
	if len(want) > 0 {
		t.Fatalf("manifest entries missing policy scopes: %#v", want)
	}
}

func TestBotNilRenderAppSkipsRegistration(t *testing.T) {
	app := fiber.New()
	RegisterPJSKBotRoutes(app, nil, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	req.Host = "localhost"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (no routes), got %d", resp.StatusCode)
	}
}

// TestBotNoiseIKRoundTrip verifies the full Noise IK encrypt→decrypt→process→encrypt→decrypt
// round trip: the client encrypts a MsgPack-encoded BotCommandRequest with Noise IK,
// the server decrypts, processes, and returns a Noise-encrypted MsgPack response.
func TestBotNoiseIKRoundTrip(t *testing.T) {
	serverKP, err := noiseCrypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate server key pair: %v", err)
	}

	var client *drawing.HarukiDrawingClient
	app := fiber.New()
	runtime := testRenderApp(t, client)
	RegisterPJSKBotRoutes(app, runtime, nil, nil, serverKP)

	// Build request payload
	cmdReq := BotCommandRequest{
		Platform:       "qq",
		PlatformUserID: "999",
		Server:         "jp",
		MatchedCommand: "/查卡",
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/查卡 1001"}},
		},
	}
	plaintext, err := noiseMP.Marshal(cmdReq)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}

	// Noise NK: client is initiator, knows server public key
	clientNC, err := noiseCrypto.NewInitiator(serverKP.Public)
	if err != nil {
		t.Fatalf("client handshake init: %v", err)
	}
	ciphertext, err := clientNC.EncryptPacket(plaintext)
	if err != nil {
		t.Fatalf("client encrypt: %v", err)
	}

	httpReq, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), bytes.NewReader(ciphertext))
	httpReq.Host = "localhost"
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	resp, err := app.Test(httpReq)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	// The raw HTTP response body is Noise-encrypted
	if len(respBody) == 0 {
		t.Fatalf("expected non-empty encrypted response")
	}

	// Decrypt the response (client reads server's Message 2)
	decrypted, err := clientNC.DecryptPacket(respBody)
	if err != nil {
		t.Fatalf("client decrypt response: %v", err)
	}

	// Decode MsgPack response
	var envelope map[string]any
	if err := noiseMP.Unmarshal(decrypted, &envelope); err != nil {
		t.Fatalf("msgpack unmarshal response: %v raw_len=%d", err, len(decrypted))
	}

	// The handler should have processed the card query
	message, _ := envelope["message"].(string)
	status := envelope["status"]
	t.Logf("Noise IK response: status=%v message=%s", status, message)

	if message != "ok" && message != "render failed" {
		t.Fatalf("unexpected message: %s (full envelope: %+v)", message, envelope)
	}
}
