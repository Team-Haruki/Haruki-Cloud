package provider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"

	"haruki-cloud/database/sekai/bondshonorword"
	"haruki-cloud/database/sekai/customprofiletextcolor"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/cachefill"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/testutil"
)

func TestDBEducationProviderMissionFillIsSharedAcrossConcurrentCallers(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "missions_singleflight")
	client := provider.client
	_, err := client.Charactermissionv2.Create().
		SetGameID(501).SetCharacterID(5).SetCharacterMissionType("leader").SetParameterGroupID(101).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx)
	testutil.Require(t, err == nil, "create mission: %v", err)

	var queries atomic.Int32
	client.Charactermissionv2.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			// Hold the fill so every caller below joins it.
			time.Sleep(100 * time.Millisecond)
			return next.Query(ctx, query)
		})
	}))

	const callers = 8
	results := make([][]*CharacterMission, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = provider.education.GetCharacterMissions(ctx, 5)
		}()
	}
	wg.Wait()

	testutil.Require(t, queries.Load() == 1, "%d concurrent callers ran %d mission queries, want 1", callers, queries.Load())
	for i, missions := range results {
		testutil.Require(t, len(missions) == 1 && missions[0].ID == 501, "caller %d missions = %+v", i, missions)
	}
}

func TestDBMySekaiRowStoreBacksOffAfterQueryError(t *testing.T) {
	ctx := context.Background()
	p := openMasterRowsProvider(t, "backoff")
	clock := testutil.NewFakeClock()
	p.mysekai.fill.Now = clock.Now
	const filename = "customProfileTextColors.json"

	_, err := p.mysekai.db.Exec("DROP TABLE customprofiletextcolors")
	testutil.Require(t, err == nil, "drop table: %v", err)
	rows, ok := p.mysekai.LoadMasterRows(ctx, filename)
	testutil.Require(t, !ok && rows == nil, "dropped table was served: %#v ok=%v", rows, ok)
	testutil.Require(t, p.mysekai.fill.Failing(filename), "table must back off after a query error")

	testutil.Require(t, p.client.Schema.Create(ctx) == nil, "recreate schema")
	_, err = p.client.Customprofiletextcolor.Create().SetGameID(10).SetSeq(1).SetColorCode("#ffffff").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create text color: %v", err)
	rows, ok = p.mysekai.LoadMasterRows(ctx, filename)
	testutil.Require(t, !ok && rows == nil, "the database must not be retried during the fill backoff: %#v ok=%v", rows, ok)

	clock.Advance(cachefill.DefaultBackoff)
	rows, ok = p.mysekai.LoadMasterRows(ctx, filename)
	testutil.Require(t, ok && len(rows) == 1 && rows[10]["colorCode"] == "#ffffff", "the call after the backoff must retry and read the database: %#v ok=%v", rows, ok)

	// A masterdata reset (the registry poll after an ingest) clears the
	// backoff along with the cached rows.
	_, err = p.mysekai.db.Exec("DROP TABLE customprofiletextcolors")
	testutil.Require(t, err == nil, "drop table again: %v", err)
	p.ResetMasterdataCache()
	_, ok = p.mysekai.LoadMasterRows(ctx, filename)
	testutil.Require(t, !ok, "dropped table was served after the reset")
	testutil.Require(t, p.client.Schema.Create(ctx) == nil, "recreate schema again")
	_, err = p.client.Customprofiletextcolor.Create().SetGameID(11).SetSeq(2).SetColorCode("#000000").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create text color again: %v", err)
	p.ResetMasterdataCache()
	rows, ok = p.mysekai.LoadMasterRows(ctx, filename)
	testutil.Require(t, ok && len(rows) == 1 && rows[11]["colorCode"] == "#000000", "a reset must clear the backoff: %#v ok=%v", rows, ok)
}

// holdAfterFirstQuery wraps an ent query so the first call completes its
// database read and then waits for release, letting a test reset the cache
// between the read and the store.
func holdAfterFirstQuery(queried chan<- struct{}, release <-chan struct{}) ent.Interceptor {
	var once sync.Once
	return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			value, err := next.Query(ctx, query)
			once.Do(func() {
				close(queried)
				<-release
			})
			return value, err
		})
	})
}

func TestDBHonorProviderBondsWordFillStraddlingResetIsDiscarded(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "bonds_words_stale")
	client := provider.client
	_, err := client.Bondshonorword.Create().SetGameID(30).SetName("Old").SetServerRegion(renderregion.JP.String()).Save(ctx)
	testutil.Require(t, err == nil, "create bonds word: %v", err)

	queried := make(chan struct{})
	release := make(chan struct{})
	client.Bondshonorword.Intercept(holdAfterFirstQuery(queried, release))

	type outcome struct {
		word *masterdata.BondsHonorWord
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		word, err := provider.honors.GetBondsHonorWordByID(ctx, 30)
		done <- outcome{word: word, err: err}
	}()

	<-queried
	// The ingest lands and the registry poll resets the cache while the
	// fill holds rows read before it.
	err = client.Bondshonorword.Update().Where(bondshonorword.GameIDEQ(30)).SetName("New").Exec(ctx)
	testutil.Require(t, err == nil, "update bonds word: %v", err)
	provider.ResetMasterdataCache()
	close(release)

	got := <-done
	testutil.Require(t, got.err == nil && got.word != nil && got.word.Name == "New", "request straddling the reset = %+v, %v; want the post-ingest row", got.word, got.err)
	provider.honors.bondsWordMu.RLock()
	loaded, cached := provider.honors.bondsWordLoaded, provider.honors.bondsWordCache[30]
	provider.honors.bondsWordMu.RUnlock()
	testutil.Require(t, loaded && cached != nil && cached.Name == "New", "cache after the reset = loaded=%v %+v; want the post-ingest row", loaded, cached)
}

func TestDBMySekaiRowStoreFillStraddlingResetIsDiscarded(t *testing.T) {
	ctx := context.Background()
	p := openMasterRowsProvider(t, "stale")
	const filename = "customProfileTextColors.json"
	_, err := p.client.Customprofiletextcolor.Create().SetGameID(10).SetSeq(1).SetColorCode("#old").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create text color: %v", err)

	queried := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	query := p.mysekai.queryTable
	p.mysekai.query = func(ctx context.Context, table string) ([]map[string]any, error) {
		rows, err := query(ctx, table)
		once.Do(func() {
			close(queried)
			<-release
		})
		return rows, err
	}

	type outcome struct {
		rows map[int]map[string]any
		ok   bool
	}
	done := make(chan outcome, 1)
	go func() {
		rows, ok := p.mysekai.LoadMasterRows(ctx, filename)
		done <- outcome{rows: rows, ok: ok}
	}()

	<-queried
	err = p.client.Customprofiletextcolor.Update().Where(customprofiletextcolor.GameIDEQ(10)).SetColorCode("#new").Exec(ctx)
	testutil.Require(t, err == nil, "update text color: %v", err)
	p.ResetMasterdataCache()
	close(release)

	got := <-done
	testutil.Require(t, got.ok && len(got.rows) == 1 && got.rows[10]["colorCode"] == "#new", "request straddling the reset = %#v ok=%v; want the post-ingest row", got.rows, got.ok)
	p.mysekai.mu.Lock()
	cached := p.mysekai.lists[filename]
	p.mysekai.mu.Unlock()
	testutil.Require(t, len(cached) == 1 && cached[0]["colorCode"] == "#new", "cache after the reset = %#v; want the post-ingest row", cached)
}
