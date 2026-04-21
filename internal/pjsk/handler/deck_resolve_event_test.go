package handler

import (
	"context"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
)

func TestResolveDeckCharacterSelectionsFallsBackJPEventRecommendToNoEventDuringPostAggregateGap(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UnixMilli()

	provider := bridgeDeckTestMasterProvider{
		region: renderregion.JP,
		events: &bridgeDeckTestEventProvider{
			events: []*masterdata.Event{
				{
					ID:          701,
					EventType:   "marathon",
					Name:        "JP Gap Current",
					StartAt:     now - int64(4*time.Hour/time.Millisecond),
					AggregateAt: now - int64(2*time.Hour/time.Millisecond),
					ClosedAt:    now + int64(2*time.Hour/time.Millisecond),
				},
				{
					ID:          702,
					EventType:   "marathon",
					Name:        "JP Next",
					StartAt:     now + int64(3*time.Hour/time.Millisecond),
					AggregateAt: now + int64(6*time.Hour/time.Millisecond),
					ClosedAt:    now + int64(7*time.Hour/time.Millisecond),
				},
			},
		},
	}

	app := &renderapp.App{
		Provider: provider,
		Providers: map[renderregion.Value]renderprovider.MasterDataProvider{
			renderregion.JP: provider,
		},
	}

	query := renderdeck.AutoQuery{
		Region:        "jp",
		RecommendType: "event",
	}
	if err := resolveDeckCharacterSelections(ctx, &query, app); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.RecommendType != "no_event" {
		t.Fatalf("expected jp event recommend to fallback to no_event, got %q", query.RecommendType)
	}
	if query.EventID != nil {
		t.Fatalf("expected event id to be cleared after jp gap fallback, got %+v", query.EventID)
	}
}

func TestResolveDeckCharacterSelectionsUsesNextEventOutsideJPDuringPostAggregateGap(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UnixMilli()

	provider := bridgeDeckTestMasterProvider{
		region: renderregion.CN,
		events: &bridgeDeckTestEventProvider{
			events: []*masterdata.Event{
				{
					ID:          801,
					EventType:   "marathon",
					Name:        "CN Gap Current",
					StartAt:     now - int64(4*time.Hour/time.Millisecond),
					AggregateAt: now - int64(2*time.Hour/time.Millisecond),
					ClosedAt:    now + int64(2*time.Hour/time.Millisecond),
				},
				{
					ID:          802,
					EventType:   "marathon",
					Name:        "CN Next",
					StartAt:     now + int64(3*time.Hour/time.Millisecond),
					AggregateAt: now + int64(6*time.Hour/time.Millisecond),
					ClosedAt:    now + int64(7*time.Hour/time.Millisecond),
				},
			},
		},
	}

	app := &renderapp.App{
		Provider: provider,
		Providers: map[renderregion.Value]renderprovider.MasterDataProvider{
			renderregion.CN: provider,
		},
	}

	query := renderdeck.AutoQuery{
		Region:        "cn",
		RecommendType: "event",
	}
	if err := resolveDeckCharacterSelections(ctx, &query, app); err != nil {
		t.Fatalf("resolveDeckCharacterSelections() error = %v", err)
	}
	if query.RecommendType != "event" {
		t.Fatalf("expected cn event recommend to stay event, got %q", query.RecommendType)
	}
	if query.EventID == nil || *query.EventID != 802 {
		t.Fatalf("expected cn gap to jump to next event 802, got %+v", query.EventID)
	}
}
