package subscription

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/config"
	pjskdb "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/accountdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type birthdayToolboxRecorder struct {
	mu       sync.Mutex
	requests map[string]int
	upserts  []sekaiapi.MysekaiBirthdayMonitorUpsertRequest
}

func newBirthdayToolboxRecorder() *birthdayToolboxRecorder {
	return &birthdayToolboxRecorder{requests: make(map[string]int)}
}

func (recorder *birthdayToolboxRecorder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	recorder.record(request)
	switch request.Method {
	case http.MethodPut:
		recorder.handleUpsert(w, request)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		recorder.handleEvent(w, request)
	case http.MethodPost:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, request)
	}
}

func (recorder *birthdayToolboxRecorder) record(request *http.Request) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.requests[request.Method]++
}

func (recorder *birthdayToolboxRecorder) handleUpsert(w http.ResponseWriter, request *http.Request) {
	var upsert sekaiapi.MysekaiBirthdayMonitorUpsertRequest
	if err := json.NewDecoder(request.Body).Decode(&upsert); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	recorder.mu.Lock()
	recorder.upserts = append(recorder.upserts, upsert)
	recorder.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (recorder *birthdayToolboxRecorder) handleEvent(w http.ResponseWriter, request *http.Request) {
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	eventID := parts[len(parts)-1]
	_ = json.NewEncoder(w).Encode(sekaiapi.MysekaiBirthdayEvent{
		EventID:             eventID,
		SubscriptionID:      request.URL.Query().Get("subscription_id"),
		SubscriptionVersion: request.URL.Query().Get("subscription_version"),
		Region:              "jp",
		UID:                 "11111111111111",
		MatchedMaterialIDs:  []int{12},
		FilteredPayload:     json.RawMessage(`{"materials":[12]}`),
	})
}

func newBirthdayBoundService(t *testing.T, toolboxHandler http.Handler) (*Service, *pjskdb.Client) {
	t.Helper()
	ctx := context.Background()
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_service_pjsk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = pjskClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:birthday_service_users_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = usersClient.Close() })

	bindings := accountdata.NewBindingService(pjskClient, identity.NewResolver(usersClient), birthdayMonitorProfileValidator{})
	if _, err := bindings.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind account: %v", err)
	}
	if err := pjskClient.UserBinding.Update().SetVerified(true).Exec(ctx); err != nil {
		t.Fatalf("verify binding: %v", err)
	}

	server := httptest.NewServer(toolboxHandler)
	t.Cleanup(server.Close)
	toolbox := sekaiapi.NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, APIToken: "test"})
	return NewServiceWithToolbox(pjskClient, bindings, toolbox), pjskClient
}

func createBirthdayMonitor(t *testing.T, service *Service) *BirthdayMonitorResult {
	t.Helper()
	result, err := service.CreateOrUpdate(
		context.Background(),
		"qq",
		"42",
		"group-1",
		"cloud-1",
		"self-1",
		"jp",
		true,
		"/烤森生日监听 钻石 四叶草 30",
		true,
	)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	return result
}

func TestBirthdayServiceCreateUpdateAndCancel(t *testing.T) {
	recorder := newBirthdayToolboxRecorder()
	service, _ := newBirthdayBoundService(t, recorder)
	first := createBirthdayMonitor(t, service)
	storeBirthdayLifecycleEvent(t, context.Background(), service, first.Subscription, []byte(`{"materials":[12]}`))

	second := createBirthdayMonitor(t, service)
	if second.Subscription.ID != first.Subscription.ID || second.Token == first.Token {
		t.Fatalf("updated subscription = %#v", second)
	}
	pending, err := service.PendingEvents(context.Background(), second.Subscription.ID)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending events after update = %#v, %v", pending, err)
	}

	cancelled, err := service.Cancel(context.Background(), "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日取消监听")
	if err != nil || cancelled.Active || cancelled.CancelledAt == nil {
		t.Fatalf("cancelled subscription = %#v, %v", cancelled, err)
	}
	assertBirthdayToolboxLifecycleCalls(t, recorder)
}

func assertBirthdayToolboxLifecycleCalls(t *testing.T, recorder *birthdayToolboxRecorder) {
	t.Helper()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.requests[http.MethodPut] != 2 || recorder.requests[http.MethodDelete] != 1 {
		t.Fatalf("toolbox calls = %#v", recorder.requests)
	}
	if len(recorder.upserts) != 2 || !slices.Equal(recorder.upserts[1].MaterialIDs, []int{12, 20}) || !recorder.upserts[1].NotifyEmpty {
		t.Fatalf("toolbox upserts = %#v", recorder.upserts)
	}
}

func TestBirthdayCreateValidationBranches(t *testing.T) {
	recorder := newBirthdayToolboxRecorder()
	service, _ := newBirthdayBoundService(t, recorder)
	ctx := context.Background()

	assertBirthdayCreateError(t, service, "", "self-1", "/烤森生日监听", "只支持群聊")
	assertBirthdayCreateError(t, service, "group-1", "", "/烤森生日监听", "缺少 OneBot self_id")
	assertBirthdayCreateError(t, service, "group-1", "self-1", "/烤森生日取消监听", "请使用取消监听接口")
	service.SetReadOnly(true)
	_, err := service.CreateOrUpdate(ctx, "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日监听", false)
	if err == nil {
		t.Fatal("read-only create succeeded")
	}
}

func assertBirthdayCreateError(t *testing.T, service *Service, groupID string, selfID string, message string, want string) {
	t.Helper()
	_, err := service.CreateOrUpdate(context.Background(), "qq", "42", groupID, "cloud-1", selfID, "jp", true, message, false)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("create error = %v, want %q", err, want)
	}
}

func TestBirthdayCancelValidationBranches(t *testing.T) {
	recorder := newBirthdayToolboxRecorder()
	service, _ := newBirthdayBoundService(t, recorder)
	ctx := context.Background()

	assertBirthdayCancelError(t, service, "group-1", "/烤森生日监听", "请使用监听接口")
	assertBirthdayCancelError(t, service, "", "/烤森生日取消监听", "只支持群聊")
	assertBirthdayCancelError(t, service, "group-1", "/烤森生日取消监听", "没有活跃")
	createBirthdayMonitor(t, service)
	assertBirthdayCancelError(t, service, "other-group", "/烤森生日取消监听", "没有由你创建")

	service.SetReadOnly(true)
	_, err := service.Cancel(ctx, "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日取消监听")
	if err == nil {
		t.Fatal("read-only cancel succeeded")
	}
}

func assertBirthdayCancelError(t *testing.T, service *Service, groupID string, message string, want string) {
	t.Helper()
	_, err := service.Cancel(context.Background(), "qq", "42", groupID, "cloud-1", "self-1", "jp", true, message)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("cancel error = %v, want %q", err, want)
	}
}

func TestBirthdayCreateRollsBackWhenToolboxSyncFails(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	})
	service, client := newBirthdayBoundService(t, handler)
	_, err := service.CreateOrUpdate(context.Background(), "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日监听", false)
	if err == nil || !strings.Contains(err.Error(), "同步 Toolbox") {
		t.Fatalf("sync error = %v", err)
	}
	subscription, queryErr := client.MysekaiBirthdaySubscription.Query().Only(context.Background())
	if queryErr != nil || subscription.Active || subscription.CancelledAt == nil {
		t.Fatalf("rolled back subscription = %#v, %v", subscription, queryErr)
	}
}

func TestBirthdayCancelKeepsActiveWhenToolboxDeleteFails(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	service, client := newBirthdayBoundService(t, handler)
	created := createBirthdayMonitor(t, service)
	_, err := service.Cancel(context.Background(), "qq", "42", "group-1", "cloud-1", "self-1", "jp", true, "/烤森生日取消监听")
	if err == nil || !strings.Contains(err.Error(), "清理 Toolbox") {
		t.Fatalf("delete error = %v", err)
	}
	subscription, queryErr := client.MysekaiBirthdaySubscription.Get(context.Background(), created.Subscription.ID)
	if queryErr != nil || !subscription.Active {
		t.Fatalf("subscription after failed delete = %#v, %v", subscription, queryErr)
	}
}

func TestBirthdayRemoteEventReadAndAck(t *testing.T) {
	recorder := newBirthdayToolboxRecorder()
	service, _ := newBirthdayBoundService(t, recorder)
	created := createBirthdayMonitor(t, service)
	ctx := context.Background()

	event, err := service.EventForClient(ctx, "event-7", fmt.Sprint(created.Subscription.ID), created.SubscriptionVersion, created.Token, "cloud-1", "group-1", "42", "self-1")
	if err != nil || event.EventID != "event-7" || event.UID != "11111111111111" || string(event.FilteredPayload) != `{"materials":[12]}` {
		t.Fatalf("remote event = %#v, %v", event, err)
	}
	if err := service.AckEvent(ctx, "event-7", fmt.Sprint(created.Subscription.ID), created.SubscriptionVersion, created.Token, "cloud-1", "group-1", "42", "self-1"); err != nil {
		t.Fatalf("remote ack: %v", err)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.requests[http.MethodGet] != 2 || recorder.requests[http.MethodPost] != 1 {
		t.Fatalf("remote event calls = %#v", recorder.requests)
	}
}
