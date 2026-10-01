package httpcoding

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func header(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Add(kv[i], kv[i+1])
	}
	return h
}

func TestAdvertisesZstd(t *testing.T) {
	cases := []struct {
		values []string
		want   bool
	}{
		{nil, false},
		{[]string{"gzip"}, false},
		{[]string{"zstd"}, true},
		{[]string{"gzip, ZSTD;q=0.5"}, true},
		{[]string{"br", "zstd"}, true},
		{[]string{"zstd;q=0"}, false},
		{[]string{"zstd;q=bogus"}, false},
		{[]string{"zstd; level=3"}, true},
	}
	for _, tc := range cases {
		h := http.Header{}
		for _, v := range tc.values {
			h.Add("Accept-Encoding", v)
		}
		if got := AdvertisesZstd(h); got != tc.want {
			t.Errorf("AdvertisesZstd(%q) = %v, want %v", tc.values, got, tc.want)
		}
	}
}

func TestNegotiatorLearnsAndWithdrawsPerOrigin(t *testing.T) {
	n := NewNegotiator()
	base := "http://100.64.0.1:3000"
	if n.Supports(base) {
		t.Fatal("unknown server must not be sent zstd")
	}
	n.Observe(base+"/state/masterdata", header("Content-Type", "application/json"))
	if n.Supports(base) {
		t.Fatal("a response without the advertisement must not enable zstd")
	}
	n.Observe(base+"/health", header("Accept-Encoding", "zstd"))
	if !n.Supports(base) || !n.Supports("HTTP://100.64.0.1:3000/") {
		t.Fatal("advertisement must enable zstd for the origin")
	}
	if n.Supports("http://100.64.0.2:3000") {
		t.Fatal("support must not leak to another origin")
	}
	now := time.Unix(1_000_000, 0)
	n.now = func() time.Time { return now }
	n.Reject(base)
	if n.Supports(base) {
		t.Fatal("a 415 must withdraw zstd")
	}
	n.Observe(base, header("Accept-Encoding", "zstd"))
	if n.Supports(base) {
		t.Fatal("an advertisement inside the cooldown must not re-enable zstd")
	}
	now = now.Add(RejectCooldown + time.Second)
	n.Observe(base, header("Accept-Encoding", "zstd"))
	if !n.Supports(base) {
		t.Fatal("an advertisement after the cooldown must re-enable zstd")
	}

	var nilNegotiator *Negotiator
	nilNegotiator.Observe(base, header("Accept-Encoding", "zstd"))
	nilNegotiator.Reject(base)
	if nilNegotiator.Supports(base) {
		t.Fatal("nil negotiator never supports zstd")
	}
}

func TestOrigin(t *testing.T) {
	if got := Origin(" http://Host:8000/api/x?y=1 "); got != "http://host:8000" {
		t.Fatalf("Origin = %q", got)
	}
	if got := Origin("not a url/"); got != "not a url" {
		t.Fatalf("Origin fallback = %q", got)
	}
}

func TestPrepareBodyOnlyForAdvertisedServersAndLargeText(t *testing.T) {
	n := NewNegotiator()
	base := "http://deck:3000"
	big := []byte(`{"data":"` + strings.Repeat("music meta ", 2000) + `"}`)

	body, coding := n.PrepareBody(base, "application/json", big)
	if coding != "" || !bytes.Equal(body, big) {
		t.Fatal("unadvertised server must get identity")
	}
	n.Observe(base, header("Accept-Encoding", "zstd"))
	body, coding = n.PrepareBody(base, "application/json; charset=utf-8", big)
	if coding != Zstd || len(body) >= len(big) {
		t.Fatalf("expected a smaller zstd body, coding=%q len=%d", coding, len(body))
	}
	decoded, err := DecodeBody(coding, body, int64(len(big)))
	if err != nil || !bytes.Equal(decoded, big) {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, coding = n.PrepareBody(base, "application/octet-stream", big); coding != "" {
		t.Fatal("octet-stream bodies are already compressed")
	}
	if _, coding = n.PrepareBody(base, "application/json", []byte(`{}`)); coding != "" {
		t.Fatal("small bodies stay identity")
	}
	random := make([]byte, 8<<10)
	for i := range random {
		random[i] = byte(i*7919 + i/3)
	}
	if body, coding = n.PrepareBody(base, "text/plain", random); coding == Zstd && len(body) >= len(random) {
		t.Fatal("an encoding that does not shrink the body must be dropped")
	}
}

func TestSetRequestHeadersAndRefusal(t *testing.T) {
	h := http.Header{}
	SetRequestHeaders(h, "")
	if h.Get("Accept-Encoding") != Zstd || h.Get("Content-Encoding") != "" {
		t.Fatalf("identity headers = %v", h)
	}
	SetRequestHeaders(h, Zstd)
	if h.Get("Content-Encoding") != Zstd {
		t.Fatalf("encoded headers = %v", h)
	}
	if !RefusedEncoding(http.StatusUnsupportedMediaType, Zstd) {
		t.Fatal("415 for an encoded body is a refusal")
	}
	if RefusedEncoding(http.StatusUnsupportedMediaType, "") || RefusedEncoding(http.StatusBadRequest, Zstd) {
		t.Fatal("only a 415 for an encoded body is a refusal")
	}
}

func TestDecodeBodyLimitsAndCodings(t *testing.T) {
	plain := []byte("identity")
	if got, err := DecodeBody("", plain, 1); err != nil || !bytes.Equal(got, plain) {
		t.Fatal("identity passes through untouched")
	}
	if got, err := DecodeBody("identity", plain, 1); err != nil || !bytes.Equal(got, plain) {
		t.Fatal("explicit identity passes through untouched")
	}
	bomb := Encode(make([]byte, 4<<20))
	if _, err := DecodeBody("zstd", bomb, 1<<20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expansion past the limit must fail, got %v", err)
	}
	if _, err := DecodeBody("zstd", bomb, 0); !errors.Is(err, ErrTooLarge) {
		t.Fatal("a non-positive limit admits nothing")
	}
	if _, err := DecodeBody("zstd", []byte("garbage"), 1<<20); err == nil {
		t.Fatal("invalid frames must fail")
	}
	if _, err := DecodeBody("gzip", plain, 1<<20); err == nil {
		t.Fatal("unrequested codings must fail")
	}
	enc, _ := zstd.NewWriter(nil)
	frame := enc.EncodeAll([]byte("exactly"), nil)
	if got, err := DecodeBody(" ZSTD ", frame, 7); err != nil || string(got) != "exactly" {
		t.Fatalf("decode at the limit failed: %v", err)
	}
}

func TestCompressible(t *testing.T) {
	if !Compressible("text/html", MinEncodeBytes) || Compressible("image/png", 1<<20) || Compressible("application/json", MinEncodeBytes-1) {
		t.Fatal("Compressible misclassified")
	}
}
