package deck

import (
	"context"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestBuildMaxProfileCardsRecordsOneEpisodeOperation(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	ctx, trace := commandtrace.WithTrace(context.Background())
	controller.ctx = ctx

	if _, err := controller.buildMaxProfileCards(renderregion.JP, 0); err != nil {
		t.Fatalf("buildMaxProfileCards() error = %v", err)
	}
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name != "deck.max_profile.episodes" {
			continue
		}
		if operation.Count != 1 {
			t.Fatalf("episode operation count = %d, want 1", operation.Count)
		}
		return
	}
	t.Fatalf("deck.max_profile.episodes was not recorded: %+v", trace.Snapshot().Operations)
}
