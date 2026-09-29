package provider

import "testing"

func TestMysekaiGateMaxLevelsPicksHighestRowPerGate(t *testing.T) {
	byID := map[int]map[int]*MysekaiGateLevel{
		1: {1: {GateID: 1, Level: 1}, 40: {GateID: 1, Level: 40}, 70: {GateID: 1, Level: 70}},
		2: {40: {GateID: 2, Level: 40}},
		6: {},
		7: {5: nil},
	}
	got := mysekaiGateMaxLevels(byID)
	if len(got) != 2 || got[1] != 70 || got[2] != 40 {
		t.Fatalf("mysekaiGateMaxLevels = %+v, want {1:70 2:40}", got)
	}
	if mysekaiGateMaxLevels(nil) != nil || mysekaiGateMaxLevels(map[int]map[int]*MysekaiGateLevel{6: {}}) != nil {
		t.Fatal("empty inputs must yield nil")
	}
}
