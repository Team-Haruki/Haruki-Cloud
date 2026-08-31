package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
)

func newSekaiResponseClient(t *testing.T, status int, body string) *HarukiSekaiAPIClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})
	client.http.SetRetryCount(0)
	return client
}

func newToolboxResponseClient(t *testing.T, status int, body string) *HarukiToolboxClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	client.http.SetRetryCount(0)
	return client
}

func TestSekaiAPINilReceiverReadMethods(t *testing.T) {
	var client *HarukiSekaiAPIClient
	if _, err := client.GetUserProfile("jp", "1"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("profile error = %v", err)
	}
	if _, err := client.GetSystem("jp"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("system error = %v", err)
	}
	if _, err := client.GetInformation("jp"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("information error = %v", err)
	}
	if _, err := client.GetMySekaiImage("jp", "image"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("image error = %v", err)
	}
	if _, err := client.GetCustomProfileCardThumbnail("jp", "image"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("profile thumbnail error = %v", err)
	}
}

func TestSekaiAPINilReceiverHousingMethods(t *testing.T) {
	var client *HarukiSekaiAPIClient
	if _, err := client.GetMySekaiHousingCompetitionList("jp", 1, false); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("housing list error = %v", err)
	}
	if _, err := client.EnterMySekaiHousingCompetitionEntry("jp", 1, 2, 3, false); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("housing entry error = %v", err)
	}
	if _, err := client.GetMySekaiHousingCompetitionBackNumberTopList("jp"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("back-number top error = %v", err)
	}
	if _, err := client.GetMySekaiHousingCompetitionBackNumberList("jp", 1); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("back-number list error = %v", err)
	}
	if _, err := client.GetMySekaiHousingThumbnail("jp", "image"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("housing thumbnail error = %v", err)
	}
}

func TestSekaiAPINilReceiverScoreMethods(t *testing.T) {
	var client *HarukiSekaiAPIClient
	if _, err := client.GetCustomMusicScorePublished("jp", "score"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("published score error = %v", err)
	}
	if _, err := client.GetCustomMusicScore("jp", "score"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("score blob error = %v", err)
	}
	if client.WithContext(context.Background()) != nil {
		t.Fatal("nil client context clone must stay nil")
	}
	if _, _, err := client.acquireTarget(); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("target error = %v", err)
	}
	if client.authReq() == nil {
		t.Fatal("nil client must still return a request")
	}
}

func TestSekaiAPIRejectsMalformedJSONResponses(t *testing.T) {
	client := newSekaiResponseClient(t, http.StatusOK, "not-json")
	if _, err := client.GetUserProfile("jp", "1"); err == nil || !strings.Contains(err.Error(), "profile response") {
		t.Fatalf("profile error = %v", err)
	}
	if _, err := client.GetSystem("jp"); err == nil || !strings.Contains(err.Error(), "system response") {
		t.Fatalf("system error = %v", err)
	}
	if _, err := client.GetInformation("jp"); err == nil || !strings.Contains(err.Error(), "information response") {
		t.Fatalf("information error = %v", err)
	}
	if _, err := client.GetCustomMusicScorePublished("jp", "score"); err == nil || !strings.Contains(err.Error(), "custom music score response") {
		t.Fatalf("custom score error = %v", err)
	}
}

func TestSekaiAPIPublishedScoreRequiresIDAndPayload(t *testing.T) {
	client := newSekaiResponseClient(t, http.StatusOK, `{}`)
	if _, err := client.GetCustomMusicScorePublished("jp", " "); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("empty score ID error = %v", err)
	}
	if _, err := client.GetCustomMusicScorePublished("jp", "missing"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing score payload error = %v", err)
	}
}

func TestSekaiAPIMapsResponseStatuses(t *testing.T) {
	if _, err := newSekaiResponseClient(t, http.StatusNotFound, "missing").GetMySekaiImage("jp", "image"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("404 error = %v", err)
	}
	if _, err := newSekaiResponseClient(t, http.StatusServiceUnavailable, "maintenance").GetMySekaiImage("jp", "image"); !errors.Is(err, ErrServerMaintenance) {
		t.Fatalf("503 error = %v", err)
	}
	_, err := newSekaiResponseClient(t, http.StatusTeapot, `{"message":"short and stout"}`).GetMySekaiImage("jp", "image")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTeapot || apiErr.Message != "short and stout" {
		t.Fatalf("418 error = %#v", err)
	}
}

func TestSekaiAPIReportsMissingBaseURL(t *testing.T) {
	if _, err := NewSekaiAPIClient(nil).GetSystem("jp"); err == nil || !strings.Contains(err.Error(), "base_url is empty") {
		t.Fatalf("nil config error = %v", err)
	}
	if _, err := NewSekaiAPIClient(&config.SekaiAPIConfig{}).GetSystem("jp"); err == nil || !strings.Contains(err.Error(), "base_url is empty") {
		t.Fatalf("empty config error = %v", err)
	}
}

func TestSekaiAPIPostHandlesBodiesAndCancellation(t *testing.T) {
	client := newSekaiResponseClient(t, http.StatusOK, `{"ok":true}`)
	body, err := client.post("/entry", map[string]bool{"entry": true})
	if err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("post body = %q, %v", body, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.WithContext(ctx).EnterMySekaiHousingCompetitionEntry("jp", 1, 2, 3, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled post error = %v", err)
	}
}

func TestSekaiAPIHousingFalseQueryValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Query().Get("isLottery") != "False" {
			t.Errorf("isLottery = %q", request.URL.Query().Get("isLottery"))
		}
		if request.Method == http.MethodPost && request.URL.Query().Get("isBackNumber") != "false" {
			t.Errorf("isBackNumber = %q", request.URL.Query().Get("isBackNumber"))
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})

	if _, err := client.GetMySekaiHousingCompetitionList("jp", 1, false); err != nil {
		t.Fatalf("housing list error = %v", err)
	}
	if _, err := client.EnterMySekaiHousingCompetitionEntry("jp", 1, 2, 3, false); err != nil {
		t.Fatalf("housing entry error = %v", err)
	}
}

func TestToolboxNilReceiverBirthdayMethods(t *testing.T) {
	var client *HarukiToolboxClient
	ctx := context.Background()
	if err := client.UpsertMysekaiBirthdayMonitor(ctx, MysekaiBirthdayMonitorUpsertRequest{}); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("upsert error = %v", err)
	}
	if err := client.DeleteMysekaiBirthdayMonitor(ctx, "1", "v1"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := client.GetMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{}); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("event error = %v", err)
	}
	if err := client.AckMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{}); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("ack error = %v", err)
	}
}

func TestToolboxNilReceiverDataMethods(t *testing.T) {
	var client *HarukiToolboxClient
	ctx := context.Background()
	if _, err := client.GetPrivateData("jp", ToolboxDataTypeSuite, 1, "qq", "2"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("private data error = %v", err)
	}
	if _, err := client.GetPrivateDataValueContext(ctx, "jp", ToolboxDataTypeSuite, 1, "qq", "2", "key"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("private value error = %v", err)
	}
	if _, err := client.GetSuiteDataContext(ctx, "jp", 1, "qq", "2"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("suite error = %v", err)
	}
	if _, err := client.GetMySekaiDataContext(ctx, "jp", 1, "qq", "2"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("mysekai error = %v", err)
	}
	if _, err := client.GetToolboxUserFastVerificationGameAccountBindingsContext(ctx, "qq", "2"); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("binding error = %v", err)
	}
}

func TestToolboxInternalRequestAcceptsNilContext(t *testing.T) {
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: "https://toolbox.invalid", APIToken: "token", UserAgent: "agent"})
	var ctx context.Context
	request, err := client.internalRequest(ctx)
	if err != nil {
		t.Fatalf("internal request error = %v", err)
	}
	if request.Context() == nil || request.Header.Get("Authorization") != "token" || request.Header.Get("User-Agent") != "agent" {
		t.Fatalf("internal request = %#v", request)
	}
}

func TestToolboxMapsPrivateDataForbiddenStatuses(t *testing.T) {
	if err := toolboxPrivateDataError(t, http.StatusForbidden, `{"message":"invalid platform or platform_user_id"}`); !errors.Is(err, ErrInvalidPlatformUser) {
		t.Fatalf("invalid platform error = %v", err)
	}
	if err := toolboxPrivateDataError(t, http.StatusForbidden, `{"message":"account owner is banned"}`); !errors.Is(err, ErrAccountOwnerBanned) {
		t.Fatalf("banned owner error = %v", err)
	}
	err := toolboxPrivateDataError(t, http.StatusForbidden, `{"message":"other forbidden"}`)
	var apiErr *ToolboxAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("generic forbidden error = %#v", err)
	}
}

func TestToolboxMapsPrivateDataNotFoundStatuses(t *testing.T) {
	if err := toolboxPrivateDataError(t, http.StatusNotFound, `{"message":"account binding not found"}`); !errors.Is(err, ErrAccountBindingNotFound) {
		t.Fatalf("binding error = %v", err)
	}
	if err := toolboxPrivateDataError(t, http.StatusNotFound, `{"message":"game data not found"}`); !errors.Is(err, ErrGameDataNotFound) {
		t.Fatalf("data error = %v", err)
	}
	err := toolboxPrivateDataError(t, http.StatusNotFound, `{"message":"other missing"}`)
	var apiErr *ToolboxAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("generic not-found error = %#v", err)
	}
}

func TestToolboxMapsPrivateDataOtherStatuses(t *testing.T) {
	err := toolboxPrivateDataError(t, http.StatusServiceUnavailable, "")
	var unavailable *ToolboxAPIError
	if !errors.As(err, &unavailable) || unavailable.Message != "toolbox service unavailable" {
		t.Fatalf("service unavailable error = %#v", err)
	}
	err = toolboxPrivateDataError(t, http.StatusTeapot, "plain failure")
	var teapot *ToolboxAPIError
	if !errors.As(err, &teapot) || teapot.StatusCode != http.StatusTeapot || teapot.Message != "plain failure" {
		t.Fatalf("teapot error = %#v", err)
	}
}

func toolboxPrivateDataError(t *testing.T, status int, body string) error {
	t.Helper()
	client := newToolboxResponseClient(t, status, body)
	_, err := client.GetPrivateDataContext(context.Background(), "jp", ToolboxDataTypeSuite, 1, "qq", "2")
	return err
}

func TestToolboxMapsBindingForbiddenStatuses(t *testing.T) {
	if err := toolboxBindingError(t, http.StatusForbidden, `{"message":"invalid platform or platform_user_id"}`); !errors.Is(err, ErrInvalidPlatformUser) {
		t.Fatalf("invalid platform error = %v", err)
	}
	if err := toolboxBindingError(t, http.StatusForbidden, `{"message":"account owner is banned"}`); !errors.Is(err, ErrAccountOwnerBanned) {
		t.Fatalf("banned owner error = %v", err)
	}
	err := toolboxBindingError(t, http.StatusForbidden, `{"message":"other forbidden"}`)
	var apiErr *ToolboxAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("generic forbidden error = %#v", err)
	}
}

func TestToolboxMapsBindingOtherStatuses(t *testing.T) {
	if err := toolboxBindingError(t, http.StatusNotFound, `{"message":"account binding not found"}`); !errors.Is(err, ErrAccountBindingNotFound) {
		t.Fatalf("binding error = %v", err)
	}
	err := toolboxBindingError(t, http.StatusNotFound, `{"message":"other missing"}`)
	var notFound *ToolboxAPIError
	if !errors.As(err, &notFound) || notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("generic not-found error = %#v", err)
	}
	err = toolboxBindingError(t, http.StatusServiceUnavailable, "")
	var unavailable *ToolboxAPIError
	if !errors.As(err, &unavailable) || unavailable.Message != "toolbox service unavailable" {
		t.Fatalf("service unavailable error = %#v", err)
	}
	err = toolboxBindingError(t, http.StatusTeapot, "plain failure")
	var teapot *ToolboxAPIError
	if !errors.As(err, &teapot) || teapot.StatusCode != http.StatusTeapot {
		t.Fatalf("teapot error = %#v", err)
	}
}

func toolboxBindingError(t *testing.T, status int, body string) error {
	t.Helper()
	client := newToolboxResponseClient(t, status, body)
	_, err := client.GetToolboxUserFastVerificationGameAccountBindingsContext(context.Background(), "qq", "2")
	return err
}

func TestToolboxRejectsMalformedBindings(t *testing.T) {
	client := newToolboxResponseClient(t, http.StatusOK, "not-json")
	_, err := client.GetToolboxUserFastVerificationGameAccountBindingsContext(context.Background(), "qq", "2")
	if err == nil || !strings.Contains(err.Error(), "failed to parse game bindings") {
		t.Fatalf("bindings error = %v", err)
	}
}

func TestToolboxBirthdayMethodsHonorCancellation(t *testing.T) {
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: "http://127.0.0.1:1"})
	client.http.SetRetryCount(0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.UpsertMysekaiBirthdayMonitor(ctx, MysekaiBirthdayMonitorUpsertRequest{SubscriptionID: "1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("upsert error = %v", err)
	}
	if err := client.DeleteMysekaiBirthdayMonitor(ctx, "1", "v1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := client.GetMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{EventID: "1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("event error = %v", err)
	}
	if err := client.AckMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{EventID: "1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ack error = %v", err)
	}
}

func TestToolboxValueAndBindingMethodsHonorCancellation(t *testing.T) {
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: "http://127.0.0.1:1"})
	client.http.SetRetryCount(0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetPrivateDataValueContext(ctx, "jp", ToolboxDataTypeSuite, 1, "qq", "2", "key"); !errors.Is(err, context.Canceled) {
		t.Fatalf("value error = %v", err)
	}
	if _, err := client.GetToolboxUserFastVerificationGameAccountBindingsContext(ctx, "qq", "2"); !errors.Is(err, context.Canceled) {
		t.Fatalf("bindings error = %v", err)
	}
}
