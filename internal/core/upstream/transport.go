package upstream

import (
	"net"
	"net/http"
	"time"
)

// TunedTransportConfig customizes NewTunedTransport. Zero values fall back to
// pooling defaults tuned for a small set of internal upstream hosts.
type TunedTransportConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	DialTimeout         time.Duration
	KeepAlive           time.Duration
}

// NewTunedTransport returns an *http.Transport configured for high-concurrency
// connection reuse. Go's default transport caps idle connections per host at 2
// (DefaultMaxIdleConnsPerHost), which forces fresh TCP/TLS handshakes under
// burst fan-out to the same upstream; this raises that ceiling and keeps warm
// connections around so repeated calls to toolbox/sekai/deck/drawing reuse a
// pooled connection instead of dialing anew.
//
// Each call returns a fresh transport; share one instance across clients that
// target the same origin to pool their connections. It is safe for concurrent
// use (an *http.Transport is a connection pool by design).
func NewTunedTransport(cfg TunedTransportConfig) *http.Transport {
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 200
	}
	perHost := cfg.MaxIdleConnsPerHost
	if perHost <= 0 {
		perHost = 32
	}
	idleTimeout := cfg.IdleConnTimeout
	if idleTimeout <= 0 {
		idleTimeout = 90 * time.Second
	}
	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	keepAlive := cfg.KeepAlive
	if keepAlive <= 0 {
		keepAlive = 30 * time.Second
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: keepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdle,
		MaxIdleConnsPerHost:   perHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
