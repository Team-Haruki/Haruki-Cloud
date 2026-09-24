package provider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/cachefill"
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
			results[i] = provider.education.GetCharacterMissions(ctx, 5)
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
