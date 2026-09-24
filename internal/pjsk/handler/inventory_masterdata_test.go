package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/cachefill"
	renderinventory "haruki-cloud/internal/pjsk/render/inventory"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/imagecache"
)

func newInventoryOutageRequestContext(t *testing.T, localDir string) *RequestContext {
	t.Helper()
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:handler_inventory_outage_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if err := sekaiClient.Close(); err != nil {
		t.Fatalf("close sekai client: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(server.Close)

	return &RequestContext{
		Ctx: ctx,
		Cmd: &CommandRequest{
			Module:            parser.ModuleMisc,
			Mode:              "inventory-list",
			Params:            []byte(`{}`),
			RequesterPlatform: "qq",
			RequesterUserID:   "42",
		},
		App: &renderapp.App{
			Config: renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true}},
			Inventory: renderinventory.NewController(drawing.NewHarukiDrawingClient(server.URL), nil, nil, renderregion.CN, renderinventory.MasterdataOptions{
				Sekai:    sekaiClient,
				LocalDir: localDir,
			}),
			Bindings:   service,
			Snapshots:  rendersnapshot.NewStaticSnapshotProvider(mustBridgeEducationSnapshot(t)),
			ImageCache: imagecache.New("https://example.com", t.TempDir()),
		},
		Region:         renderregion.CN,
		RegionStr:      "cn",
		Platform:       "qq",
		PlatformUserID: "42",
	}
}

func TestExecuteInventoryFailsWhenMasterdataDatabaseIsDown(t *testing.T) {
	rc := newInventoryOutageRequestContext(t, "")

	message, err := executeInventory(rc)
	if message != nil || !errors.Is(err, cachefill.ErrUnavailable) {
		t.Fatalf("inventory with the database down = %+v, %v; want ErrUnavailable", message, err)
	}
	replay, ok := errors.AsType[onebot11.ReplayError](WrapDomainError(err))
	if !ok || string(replay) != ErrMsgMasterdataUnavailable {
		t.Fatalf("user-facing error = %v; want %q", WrapDomainError(err), ErrMsgMasterdataUnavailable)
	}
}

func TestExecuteInventoryServesLocalMasterdataWhenDatabaseIsDown(t *testing.T) {
	root := t.TempDir()
	masterDir := filepath.Join(root, "haruki-sekai-sc-master", "master")
	if err := os.MkdirAll(masterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"materials.json", "boostItems.json", "eventItems.json", "gachaTickets.json",
		"practiceTickets.json", "skillPracticeTickets.json", "gachaCeilItems.json", "mysekaiMaterials.json",
	} {
		content := `[]`
		if name == "materials.json" {
			content = `[{"id":1,"name":"local material"}]`
		}
		if err := os.WriteFile(filepath.Join(masterDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rc := newInventoryOutageRequestContext(t, root)

	message, err := executeInventory(rc)
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("inventory with local fallback = %+v, %v; want an image", message, err)
	}
}
