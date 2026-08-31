package accountdata

import (
	"testing"

	pjskdb "haruki-cloud/database/pjsk"
)

func binding(id int, server, userID string) *pjskdb.UserBinding {
	return &pjskdb.UserBinding{ID: id, Edges: pjskdb.UserBindingEdges{GameAccount: &pjskdb.GameAccount{Server: server, UserID: userID}}}
}

func bindingWithOrder(id, displayOrder int, server, userID string) *pjskdb.UserBinding {
	return &pjskdb.UserBinding{ID: id, DisplayOrder: displayOrder, Edges: pjskdb.UserBindingEdges{GameAccount: &pjskdb.GameAccount{Server: server, UserID: userID}}}
}

func perServerBindingItems() []BindingListItem {
	return buildBindingList([]*pjskdb.UserBinding{
		binding(1, "jp", "2000"),
		binding(2, "cn", "1000"),
		binding(3, "jp", "3000"),
		binding(4, "cn", "1500"),
		binding(5, "en", "4000"),
	}, nil)
}

func TestBuildBindingListAssignsPerServerIndices(t *testing.T) {
	items := perServerBindingItems()
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}

	if items[0].Server != "jp" || items[0].Index != 1 {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].Server != "cn" || items[1].Index != 1 {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
	if items[2].Server != "jp" || items[2].Index != 2 {
		t.Fatalf("unexpected third item: %+v", items[2])
	}
	if items[3].Server != "cn" || items[3].Index != 2 {
		t.Fatalf("unexpected fourth item: %+v", items[3])
	}
	if items[4].Server != "en" || items[4].Index != 1 {
		t.Fatalf("unexpected fifth item: %+v", items[4])
	}
}

func TestSelectBindingWithinServer(t *testing.T) {
	items := perServerBindingItems()
	jpSecond, err := selectBinding(items, "u2", "jp")
	if err != nil {
		t.Fatalf("select jp u2: %v", err)
	}
	if jpSecond.Server != "jp" || jpSecond.UserID != "3000" {
		t.Fatalf("unexpected jp u2 target: %+v", jpSecond)
	}

	cnFirst, err := selectBinding(items, "u1", "cn")
	if err != nil {
		t.Fatalf("select cn u1: %v", err)
	}
	if cnFirst.Server != "cn" || cnFirst.UserID != "1000" {
		t.Fatalf("unexpected cn u1 target: %+v", cnFirst)
	}

	globalThird, err := selectBinding(items, "u3", "")
	if err != nil {
		t.Fatalf("select global u3: %v", err)
	}
	if globalThird.Server != "jp" || globalThird.UserID != "3000" {
		t.Fatalf("unexpected global u3 target: %+v", globalThird)
	}

	if _, err := selectBinding(items, "u2", "en"); err == nil {
		t.Fatalf("expected en u2 to fail")
	}
}

func TestBuildBindingListUsesDisplayOrderBeforeBindingID(t *testing.T) {
	items := buildBindingList([]*pjskdb.UserBinding{
		bindingWithOrder(1, 3, "jp", "2000"),
		bindingWithOrder(2, 1, "cn", "1000"),
		bindingWithOrder(3, 2, "jp", "3000"),
	}, nil)

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].BindingID != 2 || items[1].BindingID != 3 || items[2].BindingID != 1 {
		t.Fatalf("unexpected binding order: %+v", items)
	}
}
