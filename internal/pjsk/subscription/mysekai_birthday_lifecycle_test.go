package subscription

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
	pjskdb "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"

	_ "github.com/mattn/go-sqlite3"
)

func newBirthdayLifecycleDB(t *testing.T) *pjskdb.Client {
	t.Helper()
	client := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_lifecycle_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createBirthdayLifecycleSubscription(t *testing.T, client *pjskdb.Client) *pjskdb.MysekaiBirthdaySubscription {
	t.Helper()
	subscription, err := client.MysekaiBirthdaySubscription.Create().
		SetRegion("jp").
		SetUID("123456789012345678").
		SetPlatform("qq").
		SetPlatformUserID("user-1").
		SetPlatformGroupID("group-1").
		SetCloudBotID("cloud-1").
		SetSelfID("self-1").
		SetMaterials([]string{"diamond", "clover"}).
		SetToken("version-1.secret").
		SetActive(true).
		SetExpiresAt(time.Now().Add(time.Hour)).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	return subscription
}

func TestBirthdayServiceReadinessAndUploadLookup(t *testing.T) {
	var nilService *Service
	nilService.SetReadOnly(true)
	if nilService.Ready() {
		t.Fatal("nil service is ready")
	}
	if err := nilService.requireReady(); err == nil {
		t.Fatal("nil service passed readiness check")
	}
	if err := nilService.requireDB(); err == nil {
		t.Fatal("nil service passed database check")
	}
	if err := nilService.requireWritable(); err == nil {
		t.Fatal("nil service passed writable check")
	}

	client := newBirthdayLifecycleDB(t)
	service := NewService(client, nil)
	if service.Ready() {
		t.Fatal("service without bindings is ready")
	}
	inactive, err := service.ActiveForUpload(context.Background(), "unknown", "missing")
	if err != nil {
		t.Fatalf("inactive upload lookup: %v", err)
	}
	if inactive.Active {
		t.Fatal("missing subscription reported active")
	}

	subscription := createBirthdayLifecycleSubscription(t, client)
	active, err := service.ActiveForUpload(context.Background(), " JP ", " 123456789012345678 ")
	if err != nil {
		t.Fatalf("active upload lookup: %v", err)
	}
	if !active.Active || active.SubscriptionID != fmt.Sprint(subscription.ID) || !slices.Equal(active.MaterialIDs, []int{12, 20}) || !active.NotifyEmpty {
		t.Fatalf("active upload result = %#v", active)
	}

	service.SetReadOnly(true)
	if err := service.requireWritable(); err == nil {
		t.Fatal("read-only service passed writable check")
	}
	service.SetReadOnly(false)
}

func TestBirthdayEventLocalLifecycle(t *testing.T) {
	ctx := context.Background()
	client := newBirthdayLifecycleDB(t)
	service := NewService(client, nil)
	subscription := createBirthdayLifecycleSubscription(t, client)

	for _, id := range []string{"", "bad", "0", "-1"} {
		if _, err := service.StoreEvent(ctx, BirthdayEventPayload{SubscriptionID: id}); err == nil {
			t.Fatalf("invalid subscription id %q was accepted", id)
		}
	}
	if _, err := service.StoreEvent(ctx, BirthdayEventPayload{
		SubscriptionID: fmt.Sprint(subscription.ID),
		Region:         "en",
		UID:            subscription.UID,
	}); err == nil || !strings.Contains(err.Error(), "target does not match") {
		t.Fatalf("mismatched event error = %v", err)
	}

	payload := []byte(`{"materials":[12,20]}`)
	stored, err := service.StoreEvent(ctx, BirthdayEventPayload{
		SubscriptionID:     fmt.Sprint(subscription.ID),
		Region:             "JP",
		UID:                " " + subscription.UID + " ",
		UploadTime:         time.Now().UnixMilli(),
		MatchedMaterialIDs: []int{20, 0, 12, 20, -3},
		FilteredPayload:    payload,
	})
	if err != nil {
		t.Fatalf("store event: %v", err)
	}
	payload[0] = 'x'
	if stored.SubscriptionID != fmt.Sprint(subscription.ID) || stored.EmptyResult {
		t.Fatalf("stored event = %#v", stored)
	}

	pending, err := service.PendingEvents(ctx, subscription.ID)
	if err != nil || len(pending) != 1 || pending[0].EventID != stored.EventID {
		t.Fatalf("pending events = %#v, err=%v", pending, err)
	}
	validation, err := service.ValidateToken(ctx, fmt.Sprint(subscription.ID), "version-1", "version-1.secret")
	if err != nil || !validation.Valid || validation.SubscriptionVersion != "version-1" || len(validation.PendingEvents) != 1 {
		t.Fatalf("token validation = %#v, err=%v", validation, err)
	}

	event, err := service.EventForClient(ctx, stored.EventID, fmt.Sprint(subscription.ID), "", "version-1.secret", "cloud-1", "group-1", "user-1", "self-1")
	if err != nil {
		t.Fatalf("event for client: %v", err)
	}
	if event.UID != subscription.UID || event.PlatformUserID != "user-1" || string(event.FilteredPayload) != `{"materials":[12,20]}` {
		t.Fatalf("client event = %#v", event)
	}
	event.FilteredPayload[0] = 'x'

	if _, err := service.EventForClient(ctx, stored.EventID, fmt.Sprint(subscription.ID), "", "version-1.secret", "other", "group-1", "user-1", "self-1"); err == nil || !strings.Contains(err.Error(), "context mismatch") {
		t.Fatalf("subscription mismatch error = %v", err)
	}
	if _, err := service.EventForClient(ctx, "bad", fmt.Sprint(subscription.ID), "", "version-1.secret", "cloud-1", "group-1", "user-1", "self-1"); err == nil || !strings.Contains(err.Error(), "invalid event_id") {
		t.Fatalf("invalid event id error = %v", err)
	}

	service.SetReadOnly(true)
	if err := service.AckEvent(ctx, stored.EventID, fmt.Sprint(subscription.ID), "", "version-1.secret", "cloud-1", "group-1", "user-1", "self-1"); err == nil {
		t.Fatal("read-only acknowledgement succeeded")
	}
	service.SetReadOnly(false)
	if err := service.AckEvent(ctx, stored.EventID, fmt.Sprint(subscription.ID), "", "version-1.secret", "cloud-1", "group-1", "user-1", "self-1"); err != nil {
		t.Fatalf("ack event: %v", err)
	}
	pending, err = service.PendingEvents(ctx, subscription.ID)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending events after ack = %#v, err=%v", pending, err)
	}
	if err := service.ackPendingEvents(ctx, 0, time.Now()); err != nil {
		t.Fatalf("ack zero subscription: %v", err)
	}
	if pending, err := service.pendingEventsForSubscription(ctx, nil); err != nil || pending != nil {
		t.Fatalf("nil subscription pending events = %#v, err=%v", pending, err)
	}
}

func TestBirthdayTokenValidationFailures(t *testing.T) {
	ctx := context.Background()
	client := newBirthdayLifecycleDB(t)
	service := NewService(client, nil)
	subscription := createBirthdayLifecycleSubscription(t, client)

	for _, input := range []struct {
		id      string
		version string
		token   string
	}{
		{"bad", "", "token"},
		{"0", "", "token"},
		{fmt.Sprint(subscription.ID), "", ""},
		{fmt.Sprint(subscription.ID), "", "wrong"},
		{fmt.Sprint(subscription.ID), "wrong-version", "version-1.secret"},
	} {
		result, err := service.ValidateToken(ctx, input.id, input.version, input.token)
		if err != nil || result == nil || result.Valid {
			t.Fatalf("ValidateToken(%q, %q, %q) = %#v, %v", input.id, input.version, input.token, result, err)
		}
	}
	if _, err := service.EventForClient(ctx, "1", fmt.Sprint(subscription.ID), "", "wrong", "", "", "", ""); err == nil || !strings.Contains(err.Error(), "invalid subscription token") {
		t.Fatalf("invalid token client event error = %v", err)
	}
	if err := service.syncBirthdayMonitor(ctx, subscription.ID, "v", "jp", subscription.UID, []string{"diamond"}, time.Now(), false); err == nil {
		t.Fatal("sync without toolbox succeeded")
	}
	if err := service.deleteBirthdayMonitor(ctx, subscription.ID, "v"); err != nil {
		t.Fatalf("delete without toolbox: %v", err)
	}
}

func TestCloseBirthdayMonitorConnection(t *testing.T) {
	originalConfig := config.Cfg
	originalClient := hmesCloseHTTPClient
	t.Cleanup(func() {
		config.Cfg = originalConfig
		hmesCloseHTTPClient = originalClient
	})
	service := &Service{}

	config.Cfg.HMES.InternalBaseURL = ""
	if err := service.closeBirthdayMonitorConnection(context.Background(), 1, "version"); err != nil {
		t.Fatalf("disabled close request: %v", err)
	}

	var gotAuthorization, gotUserAgent, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		gotUserAgent = request.Header.Get("User-Agent")
		gotQuery = request.URL.Query().Get("subscription_version")
		if !strings.HasSuffix(request.URL.Path, "/internal/subscriptions/7/close") || request.Method != http.MethodPost {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	config.Cfg.HMES.InternalBaseURL = server.URL + "/"
	config.Cfg.HMES.InternalToken = "internal-token"
	config.Cfg.HMES.UserAgent = "birthday-test"
	hmesCloseHTTPClient = server.Client()
	if err := service.closeBirthdayMonitorConnection(context.Background(), 7, " version-7 "); err != nil {
		t.Fatalf("close monitor connection: %v", err)
	}
	if gotAuthorization != "internal-token" || gotUserAgent != "birthday-test" || gotQuery != "version-7" {
		t.Fatalf("close headers/query = (%q, %q, %q)", gotAuthorization, gotUserAgent, gotQuery)
	}

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer failingServer.Close()
	config.Cfg.HMES.InternalBaseURL = failingServer.URL
	config.Cfg.HMES.InternalToken = ""
	config.Cfg.HMES.UserAgent = ""
	hmesCloseHTTPClient = failingServer.Client()
	if err := service.closeBirthdayMonitorConnection(context.Background(), 7, "version-7"); err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("non-success close error = %v", err)
	}
}

func TestBirthdayMaterialAndTokenHelpers(t *testing.T) {
	if got := MaterialIDs([]string{"diamond", " diamond ", "missing", "clover"}); !slices.Equal(got, []int{12, 20}) {
		t.Fatalf("material ids = %v", got)
	}
	if got := MaterialNamesFromIDs([]int{20, 12, 999}); !slices.Equal(got, []string{"diamond", "clover"}) {
		t.Fatalf("material names = %v", got)
	}
	if token, err := randomToken(); err != nil || token == "" {
		t.Fatalf("random token = %q, %v", token, err)
	}
	version, token, err := randomVersionedToken()
	if err != nil || version == "" || tokenVersion(token) != version {
		t.Fatalf("versioned token = (%q, %q, %v)", version, token, err)
	}
	if tokenVersion("") != "" || tokenVersion("legacy-token") != "" {
		t.Fatal("legacy token unexpectedly has a version")
	}
	if normalizeRegion("") != "jp" || normalizeRegion(" TW ") != "tw" {
		t.Fatal("region normalization failed")
	}
	if selectorBindingServer("jp", false) != "" || selectorBindingServer(" jp ", true) != "jp" {
		t.Fatal("selector binding server normalization failed")
	}
	if got := normalizeMaterialIDs([]int{3, 1, 3, 0, -1, 2}); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("normalized material ids = %v", got)
	}

	now := time.Now()
	if got := normalizeUploadTime(1_700_000_000); got.Unix() != 1_700_000_000 {
		t.Fatalf("seconds upload time = %v", got)
	}
	if got := normalizeUploadTime(1_700_000_000_123); got.UnixMilli() != 1_700_000_000_123 {
		t.Fatalf("milliseconds upload time = %v", got)
	}
	if got := normalizeUploadTime(0); got.Before(now) || got.After(time.Now().Add(time.Second)) {
		t.Fatalf("default upload time = %v", got)
	}

	for _, input := range []string{"/unknown", "plain text", "/烤森生日监听 0", "/烤森生日监听 mystery"} {
		if _, err := ParseBirthdayMonitorCommand(input); err == nil {
			t.Fatalf("invalid command %q was accepted", input)
		}
	}
	if command, err := ParseBirthdayMonitorCommand("/烤森生日取消监听 20 钻石关闭"); err != nil || !command.Cancel {
		t.Fatalf("cancel command with ignored settings = %#v, %v", command, err)
	}
	if _, ok := parseDurationToken("ten"); ok {
		t.Fatal("nonnumeric duration accepted")
	}
	if !isSelectorToken("u123") || isSelectorToken("u") || isSelectorToken("user") {
		t.Fatal("selector token classification failed")
	}
}
