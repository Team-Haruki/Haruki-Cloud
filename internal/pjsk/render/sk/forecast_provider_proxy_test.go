package sk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/testutil"
)

func TestForecastProxyCarriesOnlyThirdPartySources(t *testing.T) {
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		testutil.Require(t, r.URL.Host == "forecast-source.invalid", "proxy got %s", r.URL.String())
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(proxy.Close)
	var direct atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		direct.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(local.Close)

	provider := NewRemoteForecastProviderWithConfig(ForecastConfig{LocalBaseURL: local.URL + "/", ProxyURL: " " + proxy.URL + " "})
	var out map[string]bool
	err := provider.getJSON(context.Background(), "http://forecast-source.invalid/predict.json", &out)
	testutil.Require(t, err == nil, "third-party fetch: %v", err)
	testutil.Require(t, out["ok"], "third-party body = %v", out)
	err = provider.getJSON(context.Background(), local.URL+"/prediction/cn", &out)
	testutil.Require(t, err == nil, "local fetch: %v", err)
	testutil.Require(t, proxied.Load() == 1 && direct.Load() == 1, "proxied=%d direct=%d", proxied.Load(), direct.Load())
}

func TestForecastWithoutProxySharesOneClient(t *testing.T) {
	provider := NewRemoteForecastProviderWithConfig(ForecastConfig{LocalBaseURL: "http://local.invalid"})
	testutil.Require(t, provider.clientFor("https://forecast-source.invalid/x") == provider.http, "external client should be the direct client")
	testutil.Require(t, provider.clientFor("http://local.invalid/prediction/cn") == provider.http, "local client should be the direct client")
	testutil.Require(t, (&RemoteForecastProvider{http: provider.http}).clientFor("https://forecast-source.invalid/x") == provider.http, "a literal provider falls back to http")
}
