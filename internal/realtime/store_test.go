package realtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/realtimeevent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

func TestNilStoreIsUnavailable(t *testing.T) {
	ctx := context.Background()
	for _, store := range []*Store{nil, NewStore(nil)} {
		checks := []error{}
		_, err := store.Subscription(ctx, 1)
		checks = append(checks, err)
		_, err = store.Subscriptions(ctx, []int{1})
		checks = append(checks, err)
		_, _, err = store.Insert(ctx, NewEvent{})
		checks = append(checks, err)
		_, err = store.ApplyReplayPolicy(ctx, Event{}, 1, time.Now())
		checks = append(checks, err)
		_, err = store.Pending(ctx, Key{}, 0)
		checks = append(checks, err)
		_, err = store.Claim(ctx, 1, 3, time.Now())
		checks = append(checks, err)
		_, err = store.Overdue(ctx, []int{1}, time.Now())
		checks = append(checks, err)
		_, err = store.SupersedeExhausted(ctx, 3, time.Now(), time.Now())
		checks = append(checks, err)
		_, err = store.Ack(ctx, 1, "v", "e", time.Now())
		checks = append(checks, err)
		_, err = store.SupersedeSubscription(ctx, 1, "", ReasonSubscriptionCancelled, time.Now())
		checks = append(checks, err)
		_, err = store.DeleteExpired(ctx, time.Now())
		checks = append(checks, err)
		checks = append(checks, store.Ping(ctx))
		for i, err := range checks {
			if !errors.Is(err, ErrStoreUnavailable) {
				t.Fatalf("check %d = %v, want ErrStoreUnavailable", i, err)
			}
		}
	}
}

func TestStoreEmptyInputs(t *testing.T) {
	store := NewStore(openTestDB(t))
	ctx := context.Background()
	if subs, err := store.Subscriptions(ctx, nil); err != nil || len(subs) != 0 {
		t.Fatalf("subscriptions = %v, %v", subs, err)
	}
	if events, err := store.Overdue(ctx, nil, time.Now()); err != nil || events != nil {
		t.Fatalf("overdue = %v, %v", events, err)
	}
	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestStoreInsertRejectsMissingSubscription(t *testing.T) {
	store := NewStore(openTestDB(t))
	_, created, err := store.Insert(context.Background(), NewEvent{
		SubscriptionID: 4242, SubscriptionVersion: "v1", EventID: "e", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err == nil || created || !pjskdb.IsConstraintError(err) {
		t.Fatalf("insert without subscription = %v, %v", created, err)
	}
}

type duplicateKey struct{}

// TestStoreInsertResolvesConcurrentDuplicate makes another writer insert the
// same event between the existence check and the insert.
func TestStoreInsertResolvesConcurrentDuplicate(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	event := NewEvent{SubscriptionID: sub.ID, SubscriptionVersion: "v1", EventID: "race", ExpiresAt: time.Now().Add(time.Hour)}
	env.db.RealtimeEvent.Use(func(next pjskdb.Mutator) pjskdb.Mutator {
		return pjskdb.MutateFunc(func(ctx context.Context, m pjskdb.Mutation) (pjskdb.Value, error) {
			if m.Op().Is(pjskdb.OpCreate) && ctx.Value(duplicateKey{}) == nil {
				if _, err := env.db.RealtimeEvent.Create().
					SetTopic(TopicBirthdayMonitor).
					SetSubscriptionID(event.SubscriptionID).
					SetSubscriptionVersion(event.SubscriptionVersion).
					SetEventID(event.EventID).
					SetExpiresAt(event.ExpiresAt).
					Save(context.WithValue(ctx, duplicateKey{}, true)); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	stored, created, err := env.svc.Store().Insert(context.Background(), event)
	if err != nil || created || stored.EventID != "race" {
		t.Fatalf("insert = %+v, %v, %v", stored, created, err)
	}
}

func TestStoreSupersedeSubscriptionKeepsVersion(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	store := env.svc.Store()
	ctx := context.Background()
	for _, version := range []string{"v0", "v1"} {
		if _, _, err := store.Insert(ctx, NewEvent{SubscriptionID: sub.ID, SubscriptionVersion: version, EventID: version, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if n, err := store.SupersedeSubscription(ctx, sub.ID, " v1 ", ReasonSubscriptionReplaced, time.Now()); err != nil || n != 1 {
		t.Fatalf("supersede old versions = %d, %v", n, err)
	}
	if n, err := store.SupersedeSubscription(ctx, sub.ID, "", ReasonSubscriptionCancelled, time.Now()); err != nil || n != 1 {
		t.Fatalf("supersede all = %d, %v", n, err)
	}
	rows, err := env.db.RealtimeEvent.Query().Order(realtimeevent.ByID()).All(ctx)
	if err != nil || rows[0].SupersededReason != ReasonSubscriptionReplaced || rows[1].SupersededReason != ReasonSubscriptionCancelled {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
}

func TestStoreQueriesFailOnClosedDatabase(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	_ = db.Close()
	ctx := context.Background()
	if _, err := store.Subscriptions(ctx, []int{1}); err == nil {
		t.Fatal("subscriptions on closed db")
	}
	if _, _, err := store.Insert(ctx, NewEvent{}); err == nil {
		t.Fatal("insert on closed db")
	}
	if _, err := store.ApplyReplayPolicy(ctx, Event{}, 3, time.Now()); err != nil {
		// The query error is returned.
		if !strings.Contains(err.Error(), "closed") {
			t.Fatalf("policy error = %v", err)
		}
	} else {
		t.Fatal("policy on closed db")
	}
	if _, err := store.Pending(ctx, Key{}, 0); err == nil {
		t.Fatal("pending on closed db")
	}
	if _, err := store.Overdue(ctx, []int{1}, time.Now()); err == nil {
		t.Fatal("overdue on closed db")
	}
}

// TestStoreOnPostgres runs the store against PostgreSQL when CI provides
// one: the partial index, the cascade and the unique key behave there as on
// SQLite.
func TestStoreOnPostgres(t *testing.T) {
	dsn := os.Getenv("HARUKI_IMAGECACHE_TEST_DSN")
	if dsn == "" {
		t.Skip("HARUKI_IMAGECACHE_TEST_DSN not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
		t.Fatal("requires a loopback PostgreSQL DSN")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "realtime_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		_ = admin.Close()
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	client := pjskdb.NewClient(pjskdb.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var indexDef string
	if err := admin.QueryRowContext(ctx,
		`SELECT indexdef FROM pg_indexes WHERE schemaname = $1 AND indexname = 'realtimeevent_subscription_id_subscription_version_id'`,
		schema).Scan(&indexDef); err != nil || !strings.Contains(indexDef, "WHERE") {
		t.Fatalf("partial index = %q, %v", indexDef, err)
	}

	env := &testEnv{t: t, db: client}
	sub := env.activeSubscription()
	store := NewStore(client)
	event := NewEvent{SubscriptionID: sub.ID, SubscriptionVersion: "v1", EventID: "pg", ExpiresAt: time.Now().Add(time.Hour)}
	first, created, err := store.Insert(ctx, event)
	if err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	again, created, err := store.Insert(ctx, event)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("duplicate insert = %+v, %v, %v", again, created, err)
	}
	if ok, err := store.Claim(ctx, first.ID, 3, time.Now()); err != nil || !ok {
		t.Fatalf("claim = %v, %v", ok, err)
	}
	if pending, err := store.Pending(ctx, first.Key(), 0); err != nil || len(pending) != 1 || pending[0].DeliveryCount != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	if err := client.MysekaiBirthdaySubscription.DeleteOneID(sub.ID).Exec(ctx); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	if count, err := client.RealtimeEvent.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("rows after cascade = %d, %v", count, err)
	}
}
