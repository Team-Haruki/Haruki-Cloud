package accountdata

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

var errBlockedAddress = errors.New("blocked non-public address")

// cgnatNet is the carrier-grade NAT range (100.64.0.0/10), which net.IP.IsPrivate
// does not cover but which routinely fronts internal infrastructure (e.g. tailnet).
var cgnatNet = func() *net.IPNet {
	_, n, _ := net.ParseCIDR("100.64.0.0/10")
	return n
}()

// isBlockedIP reports whether ip must not be reachable via a user-supplied URL
// (SSRF guard): loopback, private, link-local (incl. the cloud metadata address
// 169.254.169.254), CGNAT, multicast, and the unspecified address.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if cgnatNet != nil && cgnatNet.Contains(ip) {
		return true
	}
	return false
}

// safeDialContext resolves the target host and dials only public IPs, refusing
// internal/loopback/link-local/CGNAT addresses. Because it dials the exact IP it
// validated (rather than re-resolving), it also defends against DNS-rebinding and
// redirect-to-internal. TLS still uses the original hostname for SNI/verification
// (the transport sets ServerName from the request URL, not from this address).
func safeDialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if ip := net.ParseIP(host); ip != nil {
			if isBlockedIP(ip) {
				return nil, fmt.Errorf("%w: %s", errBlockedAddress, ip)
			}
			return dialer.DialContext(ctx, network, addr)
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ipAddr := range ips {
			if isBlockedIP(ipAddr.IP) {
				lastErr = fmt.Errorf("%w: %s", errBlockedAddress, ipAddr.IP)
				continue
			}
			conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if derr == nil {
				return conn, nil
			}
			lastErr = derr
		}
		if lastErr == nil {
			lastErr = errBlockedAddress
		}
		return nil, lastErr
	}
}

// newSSRFSafeClient builds an http.Client whose transport refuses to connect to
// non-public addresses — every connection, including each redirect hop, is
// validated — and that bounds the redirect chain. Only http/https are followed.
func newSSRFSafeClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext:           safeDialContext(dialer),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("blocked redirect scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}
