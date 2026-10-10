package subscription

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/realtimeevent"
	"haruki-cloud/internal/realtime"
)

type recordedClose struct {
	subscriptionID int
	version        string
}

type fakeStreamCloser struct {
	mu     sync.Mutex
	closes []recordedClose
	err    error
}

func (f *fakeStreamCloser) CloseStreams(_ context.Context, subscriptionID int, version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes = append(f.closes, recordedClose{subscriptionID, version})
	return f.err
}

func (f *fakeStreamCloser) calls() []recordedClose {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedClose(nil), f.closes...)
}

func insertRealtimeEvent(t *testing.T, client *pjskdb.Client, subscriptionID int, version string, eventID string) {
	t.Helper()
	if _, _, err := realtime.NewStore(client).Insert(context.Background(), realtime.NewEvent{
		SubscriptionID:      subscriptionID,
		SubscriptionVersion: version,
		EventID:             eventID,
		ExpiresAt:           time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("insert realtime event: %v", err)
	}
}

func realtimeRow(t *testing.T, client *pjskdb.Client, eventID string) *pjskdb.RealtimeEvent {
	t.Helper()
	row, err := client.RealtimeEvent.Query().Where(realtimeevent.EventID(eventID)).Only(context.Background())
	if err != nil {
		t.Fatalf("query realtime event %s: %v", eventID, err)
	}
	return row
}

func TestBirthdayUpdateClosesOldVersionAndSupersedesItsEvents(t *testing.T) {
	recorder := newBirthdayToolboxRecorder()
	service, client := newBirthdayBoundService(t, recorder)
	closer := &fakeStreamCloser{}
	service.SetEventStreams(closer)

	first := createBirthdayMonitor(t, service)
	if calls := closer.calls(); len(calls) != 0 {
		t.Fatalf("a new subscription closed streams: %v", calls)
	}
	insertRealtimeEvent(t, client, first.Subscription.ID, first.SubscriptionVersion, "old-version-event")

	second := createBirthdayMonitor(t, service)
	calls := closer.calls()
	if len(calls) != 1 || calls[0] != (recordedClose{first.Subscription.ID, first.SubscriptionVersion}) {
		t.Fatalf("close calls after update = %v", calls)
	}
	row := realtimeRow(t, client, "old-version-event")
	if row.SupersededAt == nil || row.SupersededReason != realtime.ReasonSubscriptionReplaced {
		t.Fatalf("old version event = %+v", row)
	}

	insertRealtimeEvent(t, client, second.Subscription.ID, second.SubscriptionVersion, "current-event")
	if _, err := service.Cancel(context.Background(), "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日取消监听"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	calls = closer.calls()
	if len(calls) != 2 || calls[1] != (recordedClose{second.Subscription.ID, second.SubscriptionVersion}) {
		t.Fatalf("close calls after cancel = %v", calls)
	}
	if row := realtimeRow(t, client, "current-event"); row.SupersededReason != realtime.ReasonSubscriptionCancelled {
		t.Fatalf("cancelled subscription event = %+v", row)
	}
}

func TestBirthdayStreamCloseFailureDoesNotFailTheCommand(t *testing.T) {
	recorder := newBirthdayToolboxRecorder()
	service, _ := newBirthdayBoundService(t, recorder)
	service.SetEventStreams(&fakeStreamCloser{err: errors.New("events role down")})
	createBirthdayMonitor(t, service)
	createBirthdayMonitor(t, service)
	if _, err := service.Cancel(context.Background(), "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日取消监听"); err != nil {
		t.Fatalf("cancel with a failing closer: %v", err)
	}
	var nilService *Service
	nilService.SetEventStreams(nil)
}

func TestBirthdayAckMarksRealtimeEventBeforeToolbox(t *testing.T) {
	var toolboxAcks int
	var mu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			mu.Lock()
			toolboxAcks++
			mu.Unlock()
			http.Error(w, "gone", http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	service, client := newBirthdayBoundService(t, handler)
	created := createBirthdayMonitor(t, service)
	insertRealtimeEvent(t, client, created.Subscription.ID, created.SubscriptionVersion, "event-9")
	ctx := context.Background()
	id := fmt.Sprint(created.Subscription.ID)

	if err := service.AckEvent(ctx, " event-9 ", id, created.SubscriptionVersion, created.Token, "cloud-1", "group-1", "42", "self-1"); err != nil {
		t.Fatalf("ack with a failing toolbox: %v", err)
	}
	if row := realtimeRow(t, client, "event-9"); row.AckedAt == nil {
		t.Fatalf("realtime event not acknowledged: %+v", row)
	}
	mu.Lock()
	if toolboxAcks == 0 {
		t.Fatal("toolbox ack was not attempted")
	}
	mu.Unlock()

	// Unknown events are acknowledged locally as a no-op.
	if err := service.AckEvent(ctx, "unknown", id, created.SubscriptionVersion, created.Token, "cloud-1", "group-1", "42", "self-1"); err != nil {
		t.Fatalf("ack unknown event: %v", err)
	}
	for name, call := range map[string]func() error{
		"wrong token": func() error {
			return service.AckEvent(ctx, "event-9", id, created.SubscriptionVersion, "wrong", "cloud-1", "group-1", "42", "self-1")
		},
		"wrong context": func() error {
			return service.AckEvent(ctx, "event-9", id, created.SubscriptionVersion, created.Token, "cloud-2", "group-1", "42", "self-1")
		},
		"blank event": func() error {
			return service.AckEvent(ctx, " ", id, created.SubscriptionVersion, created.Token, "cloud-1", "group-1", "42", "self-1")
		},
	} {
		if err := call(); err == nil {
			t.Fatalf("%s: ack succeeded", name)
		}
	}
	_ = client.Close()
	if err := service.AckEvent(ctx, "event-9", id, created.SubscriptionVersion, created.Token, "cloud-1", "group-1", "42", "self-1"); err == nil {
		t.Fatal("ack on a closed database succeeded")
	}
}

func TestBirthdayRetireStreamsToleratesStoreFailure(t *testing.T) {
	client := newBirthdayLifecycleDB(t)
	service := NewService(client, nil)
	closer := &fakeStreamCloser{}
	service.SetEventStreams(closer)
	_ = client.Close()
	service.retireStreams(context.Background(), 1, "old", "new", realtime.ReasonSubscriptionReplaced)
	service.retireStreams(context.Background(), 1, "same", "same", realtime.ReasonSubscriptionReplaced)
	if calls := closer.calls(); len(calls) != 1 || calls[0].version != "old" {
		t.Fatalf("close calls = %v", calls)
	}
}
