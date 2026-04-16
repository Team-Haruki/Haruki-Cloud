package accountdata

import (
	"testing"

	pjskdb "haruki-cloud/database/pjsk"
)

func TestBuildBindingListAssignsPerServerIndicesAndSelectsWithinServer(t *testing.T) {
	items := buildBindingList([]*pjskdb.UserBinding{
		{ID: 1, Server: "jp", UserID: "2000"},
		{ID: 2, Server: "cn", UserID: "1000"},
		{ID: 3, Server: "jp", UserID: "3000"},
		{ID: 4, Server: "cn", UserID: "1500"},
		{ID: 5, Server: "en", UserID: "4000"},
	}, nil)

	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}

	if items[0].Server != "cn" || items[0].Index != 1 {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].Server != "cn" || items[1].Index != 2 {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
	if items[2].Server != "jp" || items[2].Index != 1 {
		t.Fatalf("unexpected third item: %+v", items[2])
	}
	if items[3].Server != "jp" || items[3].Index != 2 {
		t.Fatalf("unexpected fourth item: %+v", items[3])
	}
	if items[4].Server != "en" || items[4].Index != 1 {
		t.Fatalf("unexpected fifth item: %+v", items[4])
	}

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
	if globalThird.Server != "jp" || globalThird.UserID != "2000" {
		t.Fatalf("unexpected global u3 target: %+v", globalThird)
	}

	if _, err := selectBinding(items, "u2", "en"); err == nil {
		t.Fatalf("expected en u2 to fail")
	}
}
