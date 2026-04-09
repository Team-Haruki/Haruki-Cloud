package auth

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	ent "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/requestsranking"

	_ "github.com/mattn/go-sqlite3"
)

func TestStatisticsHandlerUpdateRequestsRankingIsAtomic(t *testing.T) {
	ctx := context.Background()
	client := newBotStatisticsTestClient(t)
	defer func() { _ = client.Close() }()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	const botID = 424242
	if _, err := client.RequestsRanking.Create().SetBotID(botID).SetCounts(0).Save(ctx); err != nil {
		t.Fatalf("seed requests ranking: %v", err)
	}

	handler := NewStatisticsHandler(NewStatisticsService(client))

	const workers = 32
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			if err := handler.updateRequestsRanking(ctx, botID); err != nil {
				errCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("updateRequestsRanking returned error: %v", err)
		}
	}

	row, err := client.RequestsRanking.Query().Where(requestsranking.BotIDEQ(botID)).Only(ctx)
	if err != nil {
		t.Fatalf("load requests ranking: %v", err)
	}
	if row.Counts != workers {
		t.Fatalf("requests ranking count mismatch: got=%d want=%d", row.Counts, workers)
	}
}

func newBotStatisticsTestClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:%s?_fk=1&_busy_timeout=5000",
		filepath.Join(t.TempDir(), fmt.Sprintf("statistics_%d.db", time.Now().UnixNano())),
	)
	client, err := ent.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bot sqlite: %v", err)
	}
	return client
}
