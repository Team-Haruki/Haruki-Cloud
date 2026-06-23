package accountdata

import (
	"bytes"
	"image"
	"image/png"
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},             // loopback
		{"::1", true},                   // loopback v6
		{"10.0.0.5", true},              // private
		{"172.16.3.4", true},            // private
		{"192.168.1.1", true},           // private
		{"169.254.169.254", true},       // link-local / cloud metadata
		{"100.64.0.1", true},            // CGNAT
		{"0.0.0.0", true},               // unspecified
		{"224.0.0.1", true},             // multicast
		{"fc00::1", true},               // ULA (private v6)
		{"fe80::1", true},               // link-local v6
		{"8.8.8.8", false},              // public
		{"1.1.1.1", false},              // public
		{"100.63.255.255", false},       // just below CGNAT
		{"100.128.0.1", false},          // just above CGNAT
		{"2606:4700:4700::1111", false}, // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := isBlockedIP(ip); got != c.blocked {
			t.Errorf("isBlockedIP(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
	if !isBlockedIP(nil) {
		t.Error("isBlockedIP(nil) must be true (fail closed)")
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeBoundedImage(t *testing.T) {
	small := pngBytes(t, 5, 5)   // 25 px
	large := pngBytes(t, 50, 50) // 2500 px

	if _, err := decodeBoundedImage(small, 100); err != nil {
		t.Errorf("small image within budget should decode, got %v", err)
	}
	if _, err := decodeBoundedImage(large, 100); err == nil {
		t.Error("image exceeding the pixel budget must be rejected")
	}
	if _, err := decodeBoundedImage(large, 0); err != nil {
		t.Errorf("maxPixels<=0 disables the check, got %v", err)
	}
	if _, err := decodeBoundedImage(nil, 100); err == nil {
		t.Error("empty data must error")
	}
	if _, err := decodeBoundedImage([]byte("not an image"), 100); err == nil {
		t.Error("garbage data must error")
	}
}
