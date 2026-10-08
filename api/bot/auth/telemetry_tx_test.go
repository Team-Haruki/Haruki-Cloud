package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/database/bot"
	"haruki-cloud/database/bot/requestsranking"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/mattn/go-sqlite3"
)

// countingDriver counts statements run outside a transaction (each one an
// implicit commit) and the transactions and commits it hands out.
type countingDriver struct {
	dialect.Driver
	failTx     bool
	autocommit atomic.Int32
	txs        atomic.Int32
	commits    atomic.Int32
}

func (d *countingDriver) Exec(ctx context.Context, query string, args, v any) error {
	d.autocommit.Add(1)
	return d.Driver.Exec(ctx, query, args, v)
}

func (d *countingDriver) Query(ctx context.Context, query string, args, v any) error {
	d.autocommit.Add(1)
	return d.Driver.Query(ctx, query, args, v)
}

func (d *countingDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	d.txs.Add(1)
	if d.failTx {
		return nil, errors.New("tx unavailable")
	}
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, d: d}, nil
}

type countingTx struct {
	dialect.Tx
	d *countingDriver
}

func (t *countingTx) Commit() error {
	t.d.commits.Add(1)
	return t.Tx.Commit()
}

func newCountingBotClient(t *testing.T) (*bot.Client, *countingDriver) {
	t.Helper()
	drv, err := entsql.Open("sqlite3", fmt.Sprintf("file:telemetry_tx_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingDriver{Driver: drv}
	client := bot.NewClient(bot.Driver(counting))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	counting.autocommit.Store(0)
	counting.txs.Store(0)
	counting.commits.Store(0)
	return client, counting
}

func assertTelemetryRows(t *testing.T, client *bot.Client, botID, wantCount int) {
	t.Helper()
	ctx := context.Background()
	if count, err := client.CommandLog.Query().Count(ctx); err != nil || count != wantCount {
		t.Fatalf("command logs = %d (%v), want %d", count, err, wantCount)
	}
	ranking, err := client.RequestsRanking.Query().Where(requestsranking.BotIDEQ(botID)).Only(ctx)
	if err != nil || ranking.Counts != int64(wantCount) {
		t.Fatalf("ranking = %+v (%v), want %d", ranking, err, wantCount)
	}
	if hourly, err := client.HourlyRequests.Query().Only(ctx); err != nil || hourly.Count != wantCount {
		t.Fatalf("hourly = %+v (%v)", hourly, err)
	}
	if daily, err := client.DailyRequests.Query().Only(ctx); err != nil || daily.Count != wantCount {
		t.Fatalf("daily = %+v (%v)", daily, err)
	}
}

func TestRecordCommandTelemetryCommitsOnce(t *testing.T) {
	client, counting := newCountingBotClient(t)
	entry := CommandLogEntry{Platform: "qq", PID: "1", GID: "2", UID: "3", Command: "/card"}
	for range 2 {
		if err := RecordCommandTelemetry(context.Background(), client, 7, entry); err != nil {
			t.Fatal(err)
		}
	}
	if counting.txs.Load() != 2 || counting.commits.Load() != 2 || counting.autocommit.Load() != 0 {
		t.Fatalf("txs=%d commits=%d autocommit statements=%d; want one transaction per command",
			counting.txs.Load(), counting.commits.Load(), counting.autocommit.Load())
	}
	assertTelemetryRows(t, client, 7, 2)
}

func TestRecordCommandTelemetryFallsBackWithoutTransaction(t *testing.T) {
	client, counting := newCountingBotClient(t)
	counting.failTx = true
	entry := CommandLogEntry{Platform: "qq", PID: "1", GID: "2", UID: "3", Command: "/card"}
	if err := RecordCommandTelemetry(context.Background(), client, 9, entry); err != nil {
		t.Fatal(err)
	}
	if counting.autocommit.Load() == 0 {
		t.Fatal("the fallback writes statement by statement")
	}
	assertTelemetryRows(t, client, 9, 1)

	if err := RecordCommandTelemetry(context.Background(), nil, 9, entry); err != nil {
		t.Fatal("a nil client is a no-op")
	}
}

func TestRecordCommandTelemetryRollsBackFailedTransaction(t *testing.T) {
	client, counting := newCountingBotClient(t)
	// An over-long command fails validation inside the transaction.
	entry := CommandLogEntry{Platform: "qq", Command: strings.Repeat("x", 4096)}
	if err := RecordCommandTelemetry(context.Background(), client, 11, entry); err == nil {
		t.Fatal("expected the command log validation error")
	}
	if counting.commits.Load() != 0 {
		t.Fatal("a failed transaction must not commit")
	}
	if count, _ := client.CommandLog.Query().Count(context.Background()); count != 0 {
		t.Fatalf("command logs = %d", count)
	}
}
