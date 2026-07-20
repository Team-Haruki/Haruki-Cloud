package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	botenttest "haruki-cloud/database/bot/enttest"
	"haruki-cloud/database/bot/requestsranking"

	_ "github.com/mattn/go-sqlite3"
)

func TestCommandTelemetryDispatcherFlushesQueuedWrites(t *testing.T) {
	client := botenttest.Open(t, "sqlite3", fmt.Sprintf("file:telemetry_dispatcher_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	dispatcher := newCommandTelemetryDispatcher(client, 4)

	entry := CommandLogEntry{Platform: "qq", PID: "7", GID: "8", UID: "9", Command: "/card"}
	if !dispatcher.Enqueue(context.Background(), 7, entry) {
		t.Fatal("enqueue unexpectedly failed")
	}
	dispatcher.Close()

	if count, err := client.CommandLog.Query().Count(context.Background()); err != nil || count != 1 {
		t.Fatalf("command log count = %d, err=%v", count, err)
	}
	ranking, err := client.RequestsRanking.Query().Where(requestsranking.BotIDEQ(7)).Only(context.Background())
	if err != nil {
		t.Fatalf("query request ranking: %v", err)
	}
	if ranking.Counts != 1 {
		t.Fatalf("request count = %d", ranking.Counts)
	}
	if dispatcher.Enqueue(context.Background(), 7, entry) {
		t.Fatal("closed dispatcher accepted telemetry")
	}
}

func TestBoundedCommandLogEntryUsesSchemaByteLimits(t *testing.T) {
	entry := boundedCommandLogEntry(CommandLogEntry{
		Platform: strings.Repeat("平", 20),
		PID:      strings.Repeat("p", 1000),
		GID:      strings.Repeat("g", 1000),
		UID:      strings.Repeat("u", 1000),
		Command:  strings.Repeat("命", 100),
	})
	for name, field := range map[string]struct {
		value string
		limit int
	}{
		"platform": {entry.Platform, 32},
		"pid":      {entry.PID, 64},
		"gid":      {entry.GID, 128},
		"uid":      {entry.UID, 128},
		"command":  {entry.Command, 128},
	} {
		if len(field.value) > field.limit {
			t.Fatalf("%s byte length = %d, limit %d", name, len(field.value), field.limit)
		}
		if !utf8.ValidString(field.value) {
			t.Fatalf("%s is not valid UTF-8: %q", name, field.value)
		}
	}
}
