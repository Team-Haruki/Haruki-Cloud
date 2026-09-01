package sekai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestRestyLoggingHookBranches(t *testing.T) {
	sentinel := errors.New("network failed")
	logRestyRetry(nil, sentinel)
	logRestyError(nil, sentinel)

	request := resty.New().R()
	request.Method = http.MethodPost
	request.Attempt = 2
	response := &resty.Response{
		Request: request,
		RawResponse: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
		},
	}
	logRestyRetry(response, sentinel)
	logRestyRetry(response, nil)
	logRestyError(request, sentinel)
}

func TestRestyResponseLoggingBranches(t *testing.T) {
	if err := logRestyResponse(nil, nil); err != nil {
		t.Fatalf("nil response logging = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("failed"))
	}))
	defer server.Close()

	response, err := resty.New().R().Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if err := logRestyResponse(nil, response); err != nil {
		t.Fatalf("failed response logging = %v", err)
	}

	okResponse := &resty.Response{
		Request:     resty.New().R(),
		RawResponse: &http.Response{StatusCode: http.StatusOK},
	}
	if err := logRestyResponse(nil, okResponse); err != nil {
		t.Fatalf("fast response logging = %v", err)
	}
}
