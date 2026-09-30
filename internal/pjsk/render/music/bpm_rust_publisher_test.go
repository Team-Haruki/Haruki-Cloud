package music

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/pjsk/render/assetindex"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage"
)

type rustFixtureCountedStore struct {
	storage.Store
	gets atomic.Int64
}

func (s *rustFixtureCountedStore) Get(ctx context.Context, key storage.Key) ([]byte, error) {
	s.gets.Add(1)
	return s.Store.Get(ctx, key)
}

// The fixture is the unmodified output of the native Rust publisher, including
// its content hashes. It deliberately omits source charts so fallback GETs fail.
func TestRustPublisherManifestAndBPMConsumedByCloud(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "assetindex", "testdata", "rust-publisher"))
	if err != nil {
		t.Fatal(err)
	}
	local, err := storage.NewLocal(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	store := &rustFixtureCountedStore{Store: local}
	manager := assetindex.New(store, assetindex.Config{Enabled: true}, nil)
	defer manager.Close()
	if err := manager.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, found, known := manager.Lookup("jp-assets/startapp/thumbnail/chara/card_a.png"); !known || !found || got != "jp-assets/startapp/thumbnail/chara/Card_A.png" {
		t.Fatalf("canonical lookup=%s,%v,%v", got, found, known)
	}
	helper := assets.NewAssetHelper("", nil).WithMetadataIndex(manager)
	controller := NewController(storeChartSource(), nil, helper, nil, nil)
	controller.SetAssetReader(assets.NewAssetReader(helper, store))
	controller.SetBPMIndexSource(manager, store)
	before := store.gets.Load()
	for _, tc := range []struct {
		id         int
		difficulty string
		bpm        float64
	}{{1, "expert", 120}, {2, "master", 196}} {
		parsed, found, err := controller.loadChartBPM(context.Background(), "jp", tc.id, tc.difficulty)
		if err != nil || !found || parsed.MainBPM != tc.bpm {
			t.Fatalf("chart=%+v,%v,%v", parsed, found, err)
		}
		if tc.id == 1 && (parsed.BarCount != 3 || parsed.Duration != 4.5 || len(parsed.Events) != 2 || parsed.Events[0] != (BPMEvent{Bar: 0, BPM: 120, Duration: 3}) || parsed.Events[1] != (BPMEvent{Bar: 1.5, BPM: 240, Duration: 1.5})) {
			t.Fatalf("Rust BPM timeline changed: %+v", parsed)
		}
	}
	if _, found, err := controller.loadChartBPM(context.Background(), "jp", 3, "expert"); err != nil || found {
		t.Fatalf("missing chart=%v,%v", found, err)
	}
	if got := store.gets.Load() - before; got != 1 {
		t.Fatalf("BPM lookup downloads=%d,want one index and zero charts", got)
	}
	t.Log("Rust prepare -> native BPM -> validate inventory -> current pointer -> Cloud metadata canonical lookup -> Cloud BPM complete index: PASS (1 index GET, 0 chart GETs)")
}
