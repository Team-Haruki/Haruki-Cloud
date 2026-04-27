package config

import "time"

// Default timeouts for HTTP clients and caches.
const (
	// HTTPClientTimeout is the default timeout for outgoing HTTP requests.
	HTTPClientTimeout = 10 * time.Second

	// TrackerHTTPClientTimeout is the default timeout for tracker requests.
	TrackerHTTPClientTimeout = 20 * time.Second

	// SKForecastHTTPClientTimeout is the default timeout for SK forecast sources.
	SKForecastHTTPClientTimeout = 30 * time.Second

	// SKForecastRefreshTimeout is the timeout for one scheduled SK forecast refresh cycle.
	SKForecastRefreshTimeout = 2 * time.Minute

	// TencentIMSTimeout is the timeout for Tencent IMS content moderation requests.
	TencentIMSTimeout = 15 * time.Second

	// ProfileBGStoreTimeout is the timeout for profile background image downloads.
	ProfileBGStoreTimeout = 30 * time.Second

	// DeckRemoteEngineTimeout is the timeout for remote deck rendering engine requests.
	DeckRemoteEngineTimeout = 75 * time.Second

	// LocalRenderCacheTTL is the TTL for locally cached render results.
	LocalRenderCacheTTL = 10 * time.Minute

	// MetaRefreshInterval is the interval between master-data refresh cycles.
	MetaRefreshInterval = 30 * time.Minute
)
