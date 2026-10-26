package event

import (
	"slices"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestMemoryEventDisplaysAndFiltersAllBonusAttributes(t *testing.T) {
	source := newTestEventSource(renderregion.JP)
	event := &masterdata.Event{ID: 221, EventType: "marathon", Name: "Vivid BAD SQUAD Memories", StartAt: 100, AggregateAt: 200}
	source.events = []*masterdata.Event{event}
	source.eventsByID[221] = event
	for _, attr := range []string{"cute", "cool", "pure", "happy", "mysterious"} {
		source.bonusesByEvent[221] = append(source.bonusesByEvent[221], &masterdata.EventDeckBonus{EventID: 221, CardAttr: attr, BonusRate: 25})
		for cid := 9; cid <= 12; cid++ {
			source.gcuByID[cid] = &masterdata.GameCharacterUnit{ID: cid, GameCharacterID: cid, Unit: "street"}
			source.bonusesByEvent[221] = append(source.bonusesByEvent[221], &masterdata.EventDeckBonus{EventID: 221, CardAttr: attr, GameCharacterUnitID: cid, BonusRate: 75})
		}
	}
	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	detail, err := builder.BuildEventDetailRequest(DetailQuery{Region: renderregion.JP, EventID: 221})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cool", "cute", "happy", "mysterious", "pure"}
	if !slices.Equal(detail.EventInfo.BonusAttrs, want) {
		t.Fatalf("attributes lost or duplicated: %+v", detail.EventInfo)
	}
	if !slices.Equal(detail.EventInfo.BonusCharaID, []int{9, 10, 11, 12}) {
		t.Fatalf("characters lost or duplicated: %+v", detail.EventInfo)
	}
	if len(detail.EventAssets.EventAttrImagePaths) != 5 {
		t.Fatalf("missing attribute images: %+v", detail.EventAssets)
	}
	for i, attr := range want {
		if !strings.HasSuffix(detail.EventAssets.EventAttrImagePaths[i], "attr_icon_"+attr+".png") {
			t.Fatalf("wrong attribute image: %+v", detail.EventAssets)
		}
		list, err := builder.BuildEventListRequest(ListQuery{Region: renderregion.JP, Attr: attr, IncludePast: true, IncludeFuture: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(list.EventInfo) != 1 || list.EventInfo[0].ID != 221 {
			t.Fatalf("%s does not match memory event: %+v", attr, list.EventInfo)
		}
		if len(list.EventInfo[0].EventAttrPaths) != 5 {
			t.Fatalf("list lost attribute images: %+v", list.EventInfo[0])
		}
	}
	if builder.matchEventBonusMeta("marathon", 221, "none", "", false, "unknown") {
		t.Fatal("unknown attribute matched memory event")
	}
}
