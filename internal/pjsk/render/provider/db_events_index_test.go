package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

type sqlRecorder struct {
	mu   sync.Mutex
	logs []string
}

func (r *sqlRecorder) record(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, fmt.Sprint(args...))
}

func (r *sqlRecorder) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	logs := r.logs
	r.logs = nil
	return logs
}

func countQueries(logs []string, table string) int {
	count := 0
	for _, line := range logs {
		if strings.Contains(line, "SELECT") && strings.Contains(line, "FROM `"+table+"`") {
			count++
		}
	}
	return count
}

func openRecordedEventProvider(t *testing.T) (*DatabaseProvider, *sqlRecorder) {
	t.Helper()
	recorder := &sqlRecorder{}
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_event_index_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()),
		sekaienttest.WithOptions(sekaiDB.Debug(), sekaiDB.Log(recorder.record)))
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	rewards := json.RawMessage(`[{"id":1,"eventId":2,"fromRank":1,"toRank":1,"eventRankingRewards":[]}]`)
	for _, item := range []struct {
		id      int64
		startAt int64
		cards   []int64
	}{{1, 3000, []int64{10, 11}}, {2, 1000, []int64{11, 12}}, {3, 2000, nil}} {
		if _, err := client.Event.Create().SetGameID(item.id).SetName(fmt.Sprintf("event %d", item.id)).SetEventType("marathon").
			SetStartAt(item.startAt).SetAggregateAt(item.startAt + 100).SetEventRankingRewardRanges(rewards).SetServerRegion("jp").Save(ctx); err != nil {
			t.Fatal(err)
		}
		for _, cardID := range item.cards {
			if _, err := client.Eventcard.Create().SetGameID(item.id*100 + cardID).SetEventID(item.id).SetCardID(cardID).SetServerRegion("jp").Save(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := client.Event.Create().SetGameID(9).SetName("other region").SetStartAt(1).SetServerRegion("tw").Save(ctx); err != nil {
		t.Fatal(err)
	}
	recorder.take()
	return NewDatabaseProvider(client, renderregion.JP), recorder
}

func TestEventGetAllIsIndexedWithoutRewardRanges(t *testing.T) {
	ctx := t.Context()
	provider, recorder := openRecordedEventProvider(t)

	first := provider.events.GetAll(ctx)
	if len(first) != 3 || first[0].ID != 2 || first[1].ID != 3 || first[2].ID != 1 || first[0].Name != "event 2" || first[0].AggregateAt != 1100 {
		t.Fatalf("GetAll() = %+v", first)
	}
	logs := recorder.take()
	if countQueries(logs, "events") != 1 {
		t.Fatalf("cold GetAll queries: %v", logs)
	}
	for _, line := range logs {
		if strings.Contains(line, "event_ranking_reward_ranges") {
			t.Fatalf("GetAll loaded the unused reward ranges: %s", line)
		}
	}

	first[0].Name = "mutated"
	second := provider.events.GetAll(ctx)
	if second[0].Name != "event 2" {
		t.Fatal("caller mutation reached the cached event index")
	}
	if logs := recorder.take(); countQueries(logs, "events") != 0 {
		t.Fatalf("warm GetAll queried again: %v", logs)
	}
	// GetByID reuses the indexed rows; reward ranges still load on demand.
	if event, err := provider.events.GetByID(ctx, 1); err != nil || event.Name != "event 1" {
		t.Fatalf("GetByID() = %+v, %v", event, err)
	}
	if rewards, err := provider.events.GetRankingHonorRewards(ctx, 2); err != nil || rewards == nil {
		t.Fatalf("GetRankingHonorRewards() = %+v, %v", rewards, err)
	}

	if _, err := provider.client.Event.Create().SetGameID(4).SetName("event 4").SetStartAt(4000).SetServerRegion("jp").Save(ctx); err != nil {
		t.Fatal(err)
	}
	recorder.take()
	provider.ResetMasterdataCache()
	if all := provider.events.GetAll(ctx); len(all) != 4 || all[3].ID != 4 {
		t.Fatalf("GetAll() after reset = %+v", all)
	}
	if logs := recorder.take(); countQueries(logs, "events") != 1 {
		t.Fatalf("reset did not reload the event index: %v", logs)
	}
}

func TestEventGetByCardIDUsesCardLinkIndex(t *testing.T) {
	ctx := t.Context()
	provider, recorder := openRecordedEventProvider(t)

	for cardID, want := range map[int]int{10: 1, 11: 1, 12: 2} {
		event, err := provider.events.GetByCardID(ctx, cardID)
		if err != nil || event.ID != want {
			t.Fatalf("GetByCardID(%d) = %+v, %v; want event %d", cardID, event, err, want)
		}
	}
	if _, err := provider.events.GetByCardID(ctx, 99); err == nil || !strings.Contains(testutil.ErrorDetail(err), "query event by card 99: sekai: eventcard not found") {
		t.Fatalf("GetByCardID(missing) error = %v", err)
	}
	if logs := recorder.take(); countQueries(logs, "eventcards") != 1 {
		t.Fatalf("card links were queried more than once: %v", logs)
	}

	if _, err := provider.client.Eventcard.Create().SetGameID(999).SetEventID(3).SetCardID(99).SetServerRegion("jp").Save(ctx); err != nil {
		t.Fatal(err)
	}
	provider.ResetMasterdataCache()
	if event, err := provider.events.GetByCardID(ctx, 99); err != nil || event.ID != 3 {
		t.Fatalf("GetByCardID(99) after reset = %+v, %v", event, err)
	}
}

func TestEventIndexFailureFallsBack(t *testing.T) {
	provider, _ := openRecordedEventProvider(t)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if events := provider.events.GetAll(canceled); events != nil {
		t.Fatalf("GetAll on a failed load = %+v, want nil without local fallback", events)
	}
	if _, err := provider.events.GetByCardID(canceled, 10); err == nil {
		t.Fatal("GetByCardID on a failed load must fail")
	}
}
