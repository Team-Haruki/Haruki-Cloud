package education

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestBondSnapshotBetterBondInfoOrdering(t *testing.T) {
	builder := bondOrderingCoverageBuilder()
	tests := []struct {
		name      string
		current   drawing.BondInfo
		candidate drawing.BondInfo
		want      bool
	}{
		{name: "higher level", current: bondInfo(1, 2, 1, false), candidate: bondInfo(1, 2, 2, false), want: true},
		{name: "lower level", current: bondInfo(1, 2, 2, false), candidate: bondInfo(1, 2, 1, true), want: false},
		{name: "owned bond", current: bondInfo(1, 2, 2, false), candidate: bondInfo(1, 2, 2, true), want: true},
		{name: "unowned bond", current: bondInfo(1, 2, 2, true), candidate: bondInfo(1, 2, 2, false), want: false},
		{name: "lower base character", current: bondInfo(1, 2, 2, true), candidate: bondInfo(1, 1, 2, true), want: true},
		{name: "lower game character", current: bondInfo(1, 102, 2, true), candidate: bondInfo(1, 101, 2, true), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := builder.betterBondInfo(test.current, test.candidate); got != test.want {
				t.Fatalf("betterBondInfo() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBondSnapshotCharacterScopedOrdering(t *testing.T) {
	builder := bondOrderingCoverageBuilder()
	tests := []struct {
		name  string
		left  drawing.BondInfo
		right drawing.BondInfo
		want  bool
	}{
		{name: "owned first", left: bondInfo(2, 2, 1, true), right: bondInfo(1, 1, 1, false), want: true},
		{name: "unowned last", left: bondInfo(1, 1, 1, false), right: bondInfo(2, 2, 1, true), want: false},
		{name: "base character", left: bondInfo(2, 1, 1, true), right: bondInfo(1, 2, 1, true), want: true},
		{name: "game character", left: bondInfo(2, 101, 1, true), right: bondInfo(1, 102, 1, true), want: true},
		{name: "left character", left: bondInfo(1, 101, 1, true), right: bondInfo(2, 101, 1, true), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := builder.lessCharacterScopedBond(test.left, test.right); got != test.want {
				t.Fatalf("lessCharacterScopedBond() = %v, want %v", got, test.want)
			}
		})
	}
}

func bondOrderingCoverageBuilder() *bondSnapshotBuilder {
	return &bondSnapshotBuilder{charStyles: map[int]*GameCharacterStyle{
		1:   {GameID: 1, CharacterID: 1},
		2:   {GameID: 2, CharacterID: 2},
		101: {GameID: 101, CharacterID: 1},
		102: {GameID: 102, CharacterID: 1},
	}}
}

func bondInfo(left, right, level int, hasBond bool) drawing.BondInfo {
	return drawing.BondInfo{CharaID1: left, CharaID2: right, BondLevel: level, HasBond: hasBond}
}
