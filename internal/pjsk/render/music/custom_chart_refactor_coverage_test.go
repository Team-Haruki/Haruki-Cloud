package music

import "testing"

func TestCustomChartRefactorResidualBranches(t *testing.T) {
	connected := map[int]struct{}{1: {}}
	if chain := collectCustomChartChain(&customChartNote{ID: 1}, nil, connected); len(chain) != 0 {
		t.Fatalf("connected chart chain = %+v", chain)
	}

	notes := []customChartNote{{ID: 2, NextConnectionID: -1, PreviousConnectionID: 9}}
	chains := appendUnconnectedCustomChartChains(nil, notes, map[int]struct{}{})
	if len(chains) != 1 || len(chains[0]) != 1 || chains[0][0].ID != 2 {
		t.Fatalf("unconnected chart chains = %+v", chains)
	}

	hold := customChartHold{StartType: customChartHoldNoteGuide, EndType: customChartHoldNoteGuide}
	if customChartNoteCounts(customChartConvertedNote{Type: customChartNoteHold}, hold, true, nil) {
		t.Fatal("guide hold start counted")
	}
	if customChartNoteCounts(customChartConvertedNote{Type: customChartNoteHoldEnd}, hold, true, nil) {
		t.Fatal("guide hold end counted")
	}

	score := newCustomChartScore()
	score.notes[1] = customChartConvertedNote{ID: 1, Tick: 0}
	score.notes[2] = customChartConvertedNote{ID: 2, Tick: customChartComboTickInterval}
	if _, _, ok := customChartHoldTickBounds(score, 1, customChartHold{End: 2}); ok {
		t.Fatal("zero-length half-beat window accepted")
	}
}
