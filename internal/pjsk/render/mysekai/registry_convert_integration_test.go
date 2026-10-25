package mysekai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestConvertRecipeRegistryAndDatabaseAcrossRegions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 6 || parts[1] != "v1" || parts[2] != "master" || parts[4] != "files" {
			t.Errorf("unexpected registry path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		region := parts[3]
		if region != "jp" && region != "en" {
			t.Errorf("unexpected region %q", region)
		}
		w.Header().Set("Content-Type", "application/json")
		switch parts[5] {
		case "mysekaiConvertItems.json":
			_, _ = w.Write([]byte(`[{"id":1,"consumeResourceBoxId":1,"mysekaiConvertItemTabId":1,"mysekaiConvertItemObtainType":"resource_box","externalId":1,"convertMinutes":12,"requiredConvertFixtureLevel":1,"onceConvertLimit":10}]`))
		case "mysekaiConvertItemTabs.json":
			_, _ = w.Write([]byte(`[{"id":1,"mysekaiConvertItemTabType":"normal","seq":1}]`))
		case "materials.json":
			_, _ = fmt.Fprintf(w, `[{"id":15,"name":"%s material"}]`, region)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()
	resolver := &masterdataResolver{cache: make(map[string]masterdataSource)}
	defer resolver.Close()
	for i, region := range []string{"jp", "en"} {
		store := newTestDBMasterdataStore(t)
		store.region = region
		_, err := store.db.Exec(`CREATE TABLE resourceboxes (
			id INTEGER PRIMARY KEY AUTOINCREMENT, game_id INTEGER, server_region TEXT,
			resource_box_purpose TEXT, details TEXT
		)`)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range []struct{ purpose, details string }{
			{"mysekai_convert_consumption", fmt.Sprintf(`[{"resourceType":"coin","resourceQuantity":%d}]`, (i+1)*1000)},
			{"mysekai_convert_obtain", `[{"resourceType":"material","resourceId":15,"resourceQuantity":5}]`},
			{"mission_reward", `[{"resourceType":"coin","resourceQuantity":999999}]`},
		} {
			_, err := store.db.Exec(`INSERT INTO resourceboxes (game_id, server_region, resource_box_purpose, details) VALUES (1, ?, ?, ?)`, region, row.purpose, row.details)
			if err != nil {
				t.Fatal(err)
			}
		}
		resolver.cache[region] = newRegistryMasterdataStore(server.URL, region, store)
	}
	controller := (&Controller{resolver: resolver, defaultRegion: renderregion.JP}).WithContext(context.Background())
	for i, region := range []string{"jp", "en", "jp"} {
		request, err := controller.BuildConvertRecipeRequest(ConvertRecipeQuery{Region: region})
		if err != nil {
			t.Fatal(err)
		}
		quantity := 1000
		if i == 1 {
			quantity = 2000
		}
		for _, want := range []string{i18n.RegionLabel(region).String(), fmt.Sprintf("金币 × %d", quantity), region + " material × 5"} {
			if !strings.Contains(request.Text, want) {
				t.Errorf("%s recipe missing %q: %s", region, want, request.Text)
			}
		}
		if strings.Contains(request.Text, "999999") {
			t.Fatal("unrelated resource box included")
		}
	}
}
