package gacha

import (
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type nilEventGachaSource struct {
	*testGachaSource
}

func (s *nilEventGachaSource) GetGachaByEventID(int) (*masterdata.Gacha, error) {
	return nil, nil
}

func TestGachaResolutionRefactorGapBranches(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)
	first := &masterdata.Gacha{ID: 1, StartAt: now - 1_000, GachaPickups: []masterdata.GachaPickup{{CardID: 10}}}
	second := &masterdata.Gacha{ID: 2, StartAt: first.StartAt, GachaPickups: []masterdata.GachaPickup{{CardID: 20}}}
	source.gachas = []*masterdata.Gacha{second, first, {ID: 3, StartAt: now + 1_000}}
	source.gachaByID[first.ID] = first
	source.eventCards[10] = []int{10}
	controller := NewController(source, nil, nil)

	query, _, err := controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, NegIndex: -1})
	if err != nil || query.GachaID != second.ID {
		t.Fatalf("negative index resolution = %+v, %v", query, err)
	}
	query, _, err = controller.resolveDetailQuery(DetailQuery{Region: renderregion.JP, EventID: 10})
	if err != nil || query.GachaID != first.ID {
		t.Fatalf("event resolution = %+v, %v", query, err)
	}
	if _, err := resolveEventGachaID(&nilEventGachaSource{source}, 11); err == nil {
		t.Fatal("nil event gacha unexpectedly resolved")
	}
	if err := futureGachaError(nil); err != nil {
		t.Fatalf("nil gacha release validation = %v", err)
	}
	if got := releasedGachas([]*masterdata.Gacha{nil, {ID: 4, StartAt: now + 1_000}}, now); len(got) != 0 {
		t.Fatalf("unreleased gacha filter = %+v", got)
	}
}
