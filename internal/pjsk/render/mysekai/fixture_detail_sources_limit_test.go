package mysekai

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeFixtureFriendcodePayloadRejectsOversizedResponseSafely(t *testing.T) {
	const sensitiveMarker = "upstream-private-response-marker"
	body := `{"fixtures":[]}` + strings.Repeat(" ", int(fixtureFriendcodeMaxResponseBytes)) + sensitiveMarker

	_, err := decodeFixtureFriendcodePayload(strings.NewReader(body))
	if !errors.Is(err, errFixtureFriendcodeResponseTooLarge) {
		t.Fatalf("decode error = %v, want response size error", err)
	}
	if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("decode error leaked response content: %v", err)
	}
}

func TestDecodeFixtureFriendcodePayloadRejectsInvalidResponseSafely(t *testing.T) {
	const sensitiveMarker = "upstream-private-response-marker"

	_, err := decodeFixtureFriendcodePayload(strings.NewReader(`{"fixtures":` + sensitiveMarker))
	if !errors.Is(err, errFixtureFriendcodeInvalidResponse) {
		t.Fatalf("decode error = %v, want invalid response error", err)
	}
	if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("decode error leaked response content: %v", err)
	}
}
