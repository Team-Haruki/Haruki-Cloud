package sk

import (
	"fmt"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type staleRecordStatusTrackerSource struct {
	testTrackerSource
	status *sekaiapi.EventStatusResponse
	err    error
}

func (s staleRecordStatusTrackerSource) GetEventStatus(server string, eventID int) (*sekaiapi.EventStatusResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.status, nil
}

func TestStaleSelfRecordWarningRequiresHealthyTrackerStatus(t *testing.T) {
	now := time.Now().UTC()
	ranks := []drawing.RankInfo{
		{Rank: 100, Time: now.Add(-6 * time.Minute).UnixMilli()},
	}
	req := TrackerRankQuery{EventID: 101, Region: "jp"}

	controller := NewController(nil)
	controller.SetTrackerIntegration(staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 1, StatusDesc: "正常"},
	}, nil, nil)

	if got := controller.StaleSelfRecordWarning(req, ranks); got != StaleSelfRecordWarning {
		t.Fatalf("expected stale warning, got %q", got)
	}

	controller.SetTrackerIntegration(staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 0},
	}, nil, nil)

	if got := controller.StaleSelfRecordWarning(req, ranks); got != StaleSelfRecordWarning {
		t.Fatalf("expected stale warning for numeric healthy status, got %q", got)
	}

	controller.SetTrackerIntegration(staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 2, StatusDesc: "sekai api timeout"},
	}, nil, nil)

	if got := controller.StaleSelfRecordWarning(req, ranks); got != "" {
		t.Fatalf("expected no warning while tracker status is unhealthy, got %q", got)
	}
}

func TestStaleSelfRecordWarningIgnoresFreshRecordsAndStatusErrors(t *testing.T) {
	now := time.Now().UTC()
	req := TrackerRankQuery{EventID: 101, Region: "jp"}

	controller := NewController(nil)
	controller.SetTrackerIntegration(staleRecordStatusTrackerSource{
		status: &sekaiapi.EventStatusResponse{Status: 1, StatusDesc: "healthy"},
	}, nil, nil)

	fresh := []drawing.RankInfo{{Rank: 100, Time: now.Add(-4 * time.Minute).UnixMilli()}}
	if got := controller.StaleSelfRecordWarning(req, fresh); got != "" {
		t.Fatalf("expected no warning for fresh record, got %q", got)
	}

	controller.SetTrackerIntegration(staleRecordStatusTrackerSource{
		err: fmt.Errorf("status unavailable"),
	}, nil, nil)

	stale := []drawing.RankInfo{{Rank: 100, Time: now.Add(-6 * time.Minute).UnixMilli()}}
	if got := controller.StaleSelfRecordWarning(req, stale); got != "" {
		t.Fatalf("expected no warning when status query fails, got %q", got)
	}
}
