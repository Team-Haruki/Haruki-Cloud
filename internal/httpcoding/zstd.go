// Package httpcoding negotiates zstd HTTP content coding with internal
// services (deck-service, Drawing).
//
// Request bodies are compressed only for a server that advertised
// `Accept-Encoding: zstd` on an earlier response (RFC 7694), so a client
// talking to an older server keeps sending identity bodies. A 415 withdraws
// the advertisement for that server until it advertises again. Responses are
// requested with `Accept-Encoding: zstd` and decoded under a byte limit.
package httpcoding

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

const (
	// Zstd is the content-coding token.
	Zstd = "zstd"
	// MinEncodeBytes is the smallest request body worth compressing.
	MinEncodeBytes = 4 << 10
	// RejectCooldown is how long a server that refused a zstd body (415) is
	// sent identity even if it keeps advertising, so a misconfigured hop
	// costs one extra round trip per cooldown rather than one per request.
	RejectCooldown = 10 * time.Minute

	headerAcceptEncoding  = "Accept-Encoding"
	headerContentEncoding = "Content-Encoding"
)

// ErrTooLarge reports a response that decodes past the caller's limit.
var ErrTooLarge = errors.New("httpcoding: decoded body exceeds limit")

// encoder is shared; EncodeAll is safe for concurrent use. A nil writer
// never makes NewWriter fail.
var encoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))

type support int8

const (
	supportUnknown support = iota
	supportYes
	supportNo
)

type originState struct {
	support       support
	rejectedUntil time.Time
}

// Negotiator remembers, per server origin, whether zstd request bodies are
// accepted. The zero value is not usable; use NewNegotiator.
type Negotiator struct {
	mu      sync.RWMutex
	origins map[string]originState
	now     func() time.Time
}

// NewNegotiator returns an empty Negotiator.
func NewNegotiator() *Negotiator {
	return &Negotiator{origins: map[string]originState{}, now: time.Now}
}

// Origin reduces a base URL or request URL to scheme://host[:port].
func Origin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(strings.TrimSpace(raw), "/")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}

// Observe records what a response from the server at base advertised. An
// advertisement does not override a refusal still inside RejectCooldown.
func (n *Negotiator) Observe(base string, header http.Header) {
	if n == nil || !AdvertisesZstd(header) {
		return
	}
	origin := Origin(base)
	n.mu.Lock()
	defer n.mu.Unlock()
	if current := n.origins[origin]; current.support == supportNo && n.now().Before(current.rejectedUntil) {
		return
	}
	n.origins[origin] = originState{support: supportYes}
}

// Reject withdraws zstd for base after it refused an encoded body.
func (n *Negotiator) Reject(base string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.origins[Origin(base)] = originState{support: supportNo, rejectedUntil: n.now().Add(RejectCooldown)}
}

// Supports reports whether base has advertised zstd request bodies.
func (n *Negotiator) Supports(base string) bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.origins[Origin(base)].support == supportYes
}

// AdvertisesZstd reports whether a response's Accept-Encoding lists zstd.
func AdvertisesZstd(header http.Header) bool {
	for _, value := range header.Values(headerAcceptEncoding) {
		for item := range strings.SplitSeq(value, ",") {
			coding, params, _ := strings.Cut(item, ";")
			if !strings.EqualFold(strings.TrimSpace(coding), Zstd) {
				continue
			}
			return qualityOf(params) > 0
		}
	}
	return false
}

func qualityOf(params string) float64 {
	for param := range strings.SplitSeq(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
			if q, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return q
			}
			return 0
		}
	}
	return 1
}

// Compressible reports whether a body of this type and size should be
// encoded. Already-compressed payloads (octet-stream, images) are left alone.
func Compressible(contentType string, size int) bool {
	if size < MinEncodeBytes {
		return false
	}
	media := strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(media, "application/json") || strings.HasPrefix(media, "text/")
}

// Encode compresses a request body.
func Encode(payload []byte) []byte {
	return encoder.EncodeAll(payload, make([]byte, 0, len(payload)/4+64))
}

// PrepareBody returns the body to send and its Content-Encoding ("" for
// identity): zstd when the server at base accepts it and the body is worth it.
func (n *Negotiator) PrepareBody(base, contentType string, payload []byte) ([]byte, string) {
	if !n.Supports(base) || !Compressible(contentType, len(payload)) {
		return payload, ""
	}
	encoded := Encode(payload)
	if len(encoded) >= len(payload) {
		return payload, ""
	}
	return encoded, Zstd
}

// SetRequestHeaders adds Accept-Encoding: zstd and, for an encoded body,
// Content-Encoding.
func SetRequestHeaders(header http.Header, contentEncoding string) {
	header.Set(headerAcceptEncoding, Zstd)
	if contentEncoding != "" {
		header.Set(headerContentEncoding, contentEncoding)
	}
}

// RefusedEncoding reports whether a response refused the encoded body it was
// sent (415), so the caller should withdraw zstd and resend identity.
func RefusedEncoding(statusCode int, sentEncoding string) bool {
	return sentEncoding != "" && statusCode == http.StatusUnsupportedMediaType
}

// DecodeBody decodes a response body by its Content-Encoding. Identity bodies
// are returned as they are; a zstd body is decoded to at most limit bytes.
func DecodeBody(contentEncoding string, body []byte, limit int64) ([]byte, error) {
	coding := strings.ToLower(strings.TrimSpace(contentEncoding))
	switch coding {
	case "", "identity":
		return body, nil
	case Zstd:
		return decodeLimited(body, limit)
	default:
		return nil, fmt.Errorf("httpcoding: unsupported response Content-Encoding %q", contentEncoding)
	}
}

func decodeLimited(body []byte, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, ErrTooLarge
	}
	stream, err := zstd.NewReader(bytes.NewReader(body), zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(64<<20))
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	decoded, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, fmt.Errorf("httpcoding: invalid zstd body: %w", err)
	}
	if int64(len(decoded)) > limit {
		return nil, ErrTooLarge
	}
	return decoded, nil
}
