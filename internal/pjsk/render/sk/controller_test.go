package sk

import (
	"fmt"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	sekaiapi "haruki-cloud/utils/sekai"
)

type testTrackerSource struct{}

func (testTrackerSource) GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (testTrackerSource) TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

type testEventSource struct {
	region renderregion.Value
	events []*masterdata.Event
	byID   map[int]*masterdata.Event
}

func (s *testEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	if eventInfo, ok := s.byID[id]; ok {
		return eventInfo, nil
	}
	return nil, fmt.Errorf("event not found")
}

func (s *testEventSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	return nil, fmt.Errorf("event not found")
}

func (s *testEventSource) GetEvents() []*masterdata.Event { return s.events }

func (s *testEventSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	return nil, nil
}

func (s *testEventSource) GetEventBannerCharacterID(eventID int) (int, error) {
	return 0, fmt.Errorf("not found")
}

func (s *testEventSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return nil, nil
}

func (s *testEventSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	return nil, fmt.Errorf("not found")
}

func (s *testEventSource) GetBanEvents(charID int) []*masterdata.Event { return nil }

func (s *testEventSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom { return nil }

func (s *testEventSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return nil, fmt.Errorf("not found")
}

func TestValidateTrackerQuerySelectsCurrentEventByRegion(t *testing.T) {
	now := time.Now().UnixMilli()
	jpEvent := &masterdata.Event{
		ID:          200,
		Name:        "JP Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}
	cnEvent := &masterdata.Event{
		ID:          120,
		Name:        "CN Event",
		StartAt:     now - int64(time.Hour/time.Millisecond),
		AggregateAt: now + int64(time.Hour/time.Millisecond),
	}

	controller := NewController(nil)
	controller.SetTrackerIntegration(testTrackerSource{}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{jpEvent},
		byID:   map[int]*masterdata.Event{jpEvent.ID: jpEvent},
	}, nil)
	controller.RegisterEventSource(&testEventSource{
		region: renderregion.CN,
		events: []*masterdata.Event{cnEvent},
		byID:   map[int]*masterdata.Event{cnEvent.ID: cnEvent},
	})

	normalized, err := controller.validateTrackerQuery(TrackerRankQuery{
		Region: "cn",
		Ranks:  []int{100},
	})
	if err != nil {
		t.Fatalf("validateTrackerQuery() error = %v", err)
	}
	if normalized.EventID != cnEvent.ID {
		t.Fatalf("expected cn current event %d, got %d", cnEvent.ID, normalized.EventID)
	}
}
