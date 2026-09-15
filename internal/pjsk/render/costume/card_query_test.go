package costume

import (
	"errors"
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type cardCostumeSource struct {
	denseListTestSource
	filter Filter
	err    error
}

func (s *cardCostumeSource) FilterCostumes(filter Filter) ([]*masterdata.Costume3d, error) {
	s.filter = filter
	if s.err != nil {
		return nil, s.err
	}
	if filter.CardID != 123 || filter.PartType != "body" {
		return nil, nil
	}
	return s.costumes[:1], nil
}

func TestCardCostumeSelectsLinkedDefaultAndColorPosition(t *testing.T) {
	source := &cardCostumeSource{denseListTestSource: denseListTestSource{costumes: []*masterdata.Costume3d{
		{ID: 7001, GroupID: 700, CharacterID: 1, PartType: "body", ColorID: 1},
		{ID: 7009, GroupID: 700, CharacterID: 1, PartType: "body", ColorID: 9},
		{ID: 7003, GroupID: 700, CharacterID: 1, PartType: "body", ColorID: 3},
		{ID: 8001, GroupID: 800, CharacterID: 1, PartType: "body", ColorID: 1},
	}}}
	controller := NewController(source, nil, nil)
	for _, tt := range []struct{ position, want int }{{0, 7001}, {1, 7001}, {2, 7003}, {3, 7009}} {
		payload, err := controller.BuildCostumeDetailRequest(Query{CardID: 123, ColorPosition: tt.position})
		if err != nil {
			t.Fatal(err)
		}
		if payload.Costume.CostumeID != tt.want {
			t.Fatalf("position %d: got %d, want %d", tt.position, payload.Costume.CostumeID, tt.want)
		}
	}
	if _, err := controller.BuildCostumeDetailRequest(Query{CardID: 7001}); err == nil || err.Error() != "该卡牌没有服装" {
		t.Fatalf("costume ID must not bypass card lookup: %v", err)
	}
	if _, err := controller.BuildCostumeDetailRequest(Query{CardID: 123, ColorPosition: -1}); err == nil {
		t.Fatal("accepted negative color")
	}
	if _, err := controller.BuildCostumeDetailRequest(Query{CardID: 123, ColorPosition: 4}); err == nil {
		t.Fatal("accepted out-of-range color")
	}
	source.err = errors.New("database unavailable")
	if _, err := controller.BuildCostumeDetailRequest(Query{CardID: 123}); !errors.Is(err, source.err) {
		t.Fatalf("query error hidden: %v", err)
	}
}
