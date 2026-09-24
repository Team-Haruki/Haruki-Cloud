package inventory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/cachefill"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func TestInventoryMasterdataReadsDatabaseFirstWithLocalFallbackPerTable(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:inventory_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))

	_, err := client.Material.Create().SetGameID(5).SetSeq(5).SetMaterialType("common").SetName("db material").SetFlavorText("from db").SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create material: %v", err)
	_, err = client.Material.Create().SetGameID(6).SetName("other region").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create jp material: %v", err)
	_, err = client.Boostitem.Create().SetGameID(2).SetName("db boost").SetRecoveryValue(10).SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create boost item: %v", err)
	_, err = client.Practiceticket.Create().SetGameID(10101).SetName("db ticket").SetCharacterID(1).SetExp(1000).SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create practice ticket: %v", err)
	_, err = client.Mysekaimaterial.Create().SetGameID(6).SetSeq(6).SetMysekaiMaterialType("mineral").SetName("db mineral").SetDescription("d").SetIconAssetbundleName("item_mineral_1").SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create mysekai material: %v", err)

	root := t.TempDir()
	masterDir := filepath.Join(root, "haruki-sekai-sc-master", "master")
	testutil.Require(t, os.MkdirAll(masterDir, 0o755) == nil, "mkdir master dir")
	for name, content := range map[string]string{
		"materials.json":      `[{"id":5,"name":"local material"}]`,
		"eventItems.json":     `[{"id":7,"eventId":70,"name":"local event item","assetbundleName":"badge"}]`,
		"gachaTickets.json":   `[{"id":8,"name":"local gacha ticket","assetbundleName":"gacha_ticket"}]`,
		"gachaCeilItems.json": `[{"id":9,"name":"local ceil item","assetbundleName":"ceil_item"}]`,
	} {
		testutil.Require(t, os.WriteFile(filepath.Join(masterDir, name), []byte(content), 0o644) == nil, "write %s", name)
	}

	store := newMasterdataStore(client, root)
	md := store.forRegion(ctx, renderregion.CN)
	testutil.Require(t, md.materials[5].Name == "db material" && md.materials[5].FlavorText == "from db" && md.materials[5].MaterialType == "common", "materials = %+v", md.materials)
	_, leaked := md.materials[6]
	testutil.Require(t, !leaked, "jp material leaked into cn: %+v", md.materials)
	testutil.Require(t, md.boostItems[2].RecoveryValue == 10 && md.boostItems[2].Name == "db boost", "boost items = %+v", md.boostItems)
	testutil.Require(t, md.practiceTickets[10101].Exp == 1000 && md.practiceTickets[10101].CharacterID == 1, "practice tickets = %+v", md.practiceTickets)
	testutil.Require(t, md.mysekaiMaterials[6].Type == "mineral" && md.mysekaiMaterials[6].IconAssetbundleName == "item_mineral_1", "mysekai materials = %+v", md.mysekaiMaterials)
	// Tables the database serves empty come from the local files.
	testutil.Require(t, md.eventItems[7].Name == "local event item" && md.eventItems[7].EventID == 70, "event items = %+v", md.eventItems)
	testutil.Require(t, md.gachaTickets[8].AssetbundleName == "gacha_ticket", "gacha tickets = %+v", md.gachaTickets)
	testutil.Require(t, md.gachaCeilItems[9].Name == "local ceil item", "gacha ceil items = %+v", md.gachaCeilItems)
	testutil.Require(t, len(md.skillPracticeTickets) == 0, "skill practice tickets = %+v", md.skillPracticeTickets)

	// Cached per region until reset.
	_, err = client.Material.Create().SetGameID(50).SetName("later").SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create later material: %v", err)
	_, cachedLater := store.forRegion(ctx, renderregion.CN).materials[50]
	testutil.Require(t, !cachedLater, "region was reloaded without a reset")
	store.resetCache()
	_, reloaded := store.forRegion(ctx, renderregion.CN).materials[50]
	testutil.Require(t, reloaded, "reset did not reload the region")

	// Without a local directory an empty table stays empty.
	dbOnly := newMasterdataStore(client, "")
	testutil.Require(t, len(dbOnly.forRegion(ctx, renderregion.CN).eventItems) == 0, "db-only store read local files")
}

func TestInventoryMasterdataDoesNotCacheFailedFills(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:inventory_fail_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	testutil.Require(t, client.Close() == nil, "close client")

	store := newMasterdataStore(client, "")
	md := store.forRegion(ctx, renderregion.JP)
	testutil.Require(t, md != nil && len(md.materials) == 0, "closed database served rows: %+v", md.materials)
	store.mu.RLock()
	cached := len(store.cache)
	store.mu.RUnlock()
	testutil.Require(t, cached == 0, "failed fill was cached: %d regions", cached)
}

func TestInventoryMasterdataSharesOneFillAcrossConcurrentRequests(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:inventory_flight_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	_, err := client.Material.Create().SetGameID(5).SetName("db material").SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create material: %v", err)

	var queries atomic.Int32
	client.Material.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			// Hold the fill so every request below joins it.
			time.Sleep(100 * time.Millisecond)
			return next.Query(ctx, query)
		})
	}))

	store := newMasterdataStore(client, "")
	const requests = 8
	results := make([]*regionMasterdata, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = store.forRegion(ctx, renderregion.CN)
		}()
	}
	wg.Wait()

	testutil.Require(t, queries.Load() == 1, "%d concurrent requests ran %d material queries, want 1", requests, queries.Load())
	for i, md := range results {
		testutil.Require(t, md != nil && md.materials[5].Name == "db material", "request %d materials = %+v", i, md)
	}
}

func TestInventoryMasterdataBacksOffAfterFailedFill(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:inventory_backoff_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	var queries atomic.Int32
	client.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, query)
		})
	}))
	testutil.Require(t, client.Close() == nil, "close client")

	root := t.TempDir()
	masterDir := filepath.Join(root, "haruki-sekai-sc-master", "master")
	testutil.Require(t, os.MkdirAll(masterDir, 0o755) == nil, "mkdir master dir")
	testutil.Require(t, os.WriteFile(filepath.Join(masterDir, "materials.json"), []byte(`[{"id":5,"name":"local material"}]`), 0o644) == nil, "write materials")

	store := newMasterdataStore(client, root)
	clock := testutil.NewFakeClock()
	store.fill.Now = clock.Now

	md := store.forRegion(ctx, renderregion.CN)
	testutil.Require(t, md != nil && md.materials[5].Name == "local material", "failed fill must serve the local files: %+v", md)
	perFill := queries.Load()
	testutil.Require(t, perFill == 8, "first fill ran %d queries, want one per table", perFill)
	store.mu.RLock()
	cached := len(store.cache)
	store.mu.RUnlock()
	testutil.Require(t, cached == 0, "failed fill was cached: %d regions", cached)

	again := store.forRegion(ctx, renderregion.CN)
	testutil.Require(t, again == md, "requests during the backoff must serve the partial fill")
	testutil.Require(t, queries.Load() == perFill, "requests during the backoff ran %d more queries", queries.Load()-perFill)

	clock.Advance(cachefill.DefaultBackoff)
	_ = store.forRegion(ctx, renderregion.CN)
	testutil.Require(t, queries.Load() == 2*perFill, "the request after the backoff must retry: %d queries", queries.Load())

	store.resetCache()
	_ = store.forRegion(ctx, renderregion.CN)
	testutil.Require(t, queries.Load() == 3*perFill, "a reset must clear the backoff: %d queries", queries.Load())
}

func TestInventoryMasterdataFillStraddlingResetIsDiscarded(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:inventory_stale_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	_, err := client.Material.Create().SetGameID(5).SetName("old").SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create material: %v", err)

	queried := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	client.Material.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			// Complete the read, then hold the fill so the reset lands
			// between the read and the store.
			value, err := next.Query(ctx, query)
			once.Do(func() {
				close(queried)
				<-release
			})
			return value, err
		})
	}))

	store := newMasterdataStore(client, "")
	done := make(chan *regionMasterdata, 1)
	go func() { done <- store.forRegion(ctx, renderregion.CN) }()

	<-queried
	_, err = client.Material.Create().SetGameID(50).SetName("ingested").SetServerRegion("cn").Save(ctx)
	testutil.Require(t, err == nil, "create ingested material: %v", err)
	store.resetCache()
	close(release)

	md := <-done
	_, served := md.materials[50]
	testutil.Require(t, served, "request straddling the reset must serve the post-ingest rows: %+v", md.materials)
	store.mu.RLock()
	cached := store.cache[renderregion.CN.String()]
	store.mu.RUnlock()
	_, cachedIngested := cached.materials[50]
	testutil.Require(t, cached != nil && cachedIngested, "cache after the reset = %+v; want the post-ingest rows", cached)
}
