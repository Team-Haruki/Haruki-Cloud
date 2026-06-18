package sk

import (
	"fmt"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type staleRecordStatusTrackerSource struct {
	testTrackerSource
	status      *sekaiapi.EventStatusResponse
	err         error
	wantEventID int
}

func (s staleRecordStatusTrackerSource) GetEventStatus(server string, eventID int) (*sekaiapi.EventStatusResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.wantEventID > 0 && eventID != s.wantEventID {
		return nil, fmt.Errorf("unexpected event id: got %d want %d", eventID, s.wantEventID)
	}
	return s.status, nil
}

func TestStaleSelfRecordWarningRequiresHealthyTrackerStatus(t *testing.T) {
	now := time.Now().UTC()
	userID := int64(1234567890)
	ranks := []drawing.RankInfo{
		{Rank: 101, Time: now.Add(-6 * time.Minute).UnixMilli()},
	}
	req := TrackerRankQuery{EventID: 101, Region: "jp", UserID: &userID}

	controller := NewController(nil)
	setTestTrackerIntegration(controller, staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 1, StatusDesc: "正常"},
	}, nil, nil)

	if got := controller.StaleSelfRecordWarning(req, ranks); got != StaleSelfRecordWarning {
		t.Fatalf("expected stale warning, got %q", got)
	}

	setTestTrackerIntegration(controller, staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 0},
	}, nil, nil)

	if got := controller.StaleSelfRecordWarning(req, ranks); got != StaleSelfRecordWarning {
		t.Fatalf("expected stale warning for numeric healthy status, got %q", got)
	}

	setTestTrackerIntegration(controller, staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 2, StatusDesc: "sekai api timeout"},
	}, nil, nil)

	if got := controller.StaleSelfRecordWarning(req, ranks); got != "" {
		t.Fatalf("expected no warning while tracker status is unhealthy, got %q", got)
	}
}

func TestStaleSelfRecordWarningIgnoresFreshRecordsAndStatusErrors(t *testing.T) {
	now := time.Now().UTC()
	userID := int64(1234567890)
	req := TrackerRankQuery{EventID: 101, Region: "jp", UserID: &userID}

	controller := NewController(nil)
	setTestTrackerIntegration(controller, staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 1, StatusDesc: "healthy"},
	}, nil, nil)

	fresh := []drawing.RankInfo{{Rank: 100, Time: now.Add(-4 * time.Minute).UnixMilli()}}
	if got := controller.StaleSelfRecordWarning(req, fresh); got != "" {
		t.Fatalf("expected no warning for fresh record, got %q", got)
	}

	inTop100 := []drawing.RankInfo{{Rank: 100, Time: now.Add(-6 * time.Minute).UnixMilli()}}
	if got := controller.StaleSelfRecordWarning(req, inTop100); got != "" {
		t.Fatalf("expected no warning for top-100 stale record, got %q", got)
	}

	setTestTrackerIntegration(controller, staleRecordStatusTrackerSource{
		err: fmt.Errorf("status unavailable"),
	}, nil, nil)

	stale := []drawing.RankInfo{{Rank: 101, Time: now.Add(-6 * time.Minute).UnixMilli()}}
	if got := controller.StaleSelfRecordWarning(req, stale); got != "" {
		t.Fatalf("expected no warning when status query fails, got %q", got)
	}
}

func TestStaleSelfRecordWarningInfersCurrentEvent(t *testing.T) {
	now := time.Now().UTC()
	userID := int64(1234567890)
	ranks := []drawing.RankInfo{
		{Rank: 101, Time: now.Add(-6 * time.Minute).UnixMilli()},
	}
	controller := NewController(nil)
	setTestTrackerIntegration(controller, staleRecordStatusTrackerSource{
		status:      &sekaiapi.EventStatusResponse{Status: 1, StatusDesc: "正常"},
		wantEventID: 202,
	}, &testEventSource{
		region: renderregion.JP,
		events: []*masterdata.Event{
			{
				ID:          202,
				StartAt:     now.Add(-time.Hour).UnixMilli(),
				AggregateAt: now.Add(time.Hour).UnixMilli(),
				ClosedAt:    now.Add(2 * time.Hour).UnixMilli(),
			},
		},
		byID: map[int]*masterdata.Event{
			202: {
				ID:          202,
				StartAt:     now.Add(-time.Hour).UnixMilli(),
				AggregateAt: now.Add(time.Hour).UnixMilli(),
				ClosedAt:    now.Add(2 * time.Hour).UnixMilli(),
			},
		},
	}, nil)

	req := TrackerRankQuery{Region: "jp", UserID: &userID}
	if got := controller.StaleSelfRecordWarning(req, ranks); got != StaleSelfRecordWarning {
		t.Fatalf("expected stale warning with inferred event id, got %q", got)
	}
}
