package pjsk

import (
	"net/http"
	"testing"

	onebot11 "haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
)

func TestApplyLegacyRequesterContextSetsRequesterIdentity(t *testing.T) {
	resolved := &parser.ResolvedCommand{}
	req := CommandRequest{
		IMPlatform: "qq",
		IMUserID:   "12345",
	}

	applyLegacyRequesterContext(resolved, req)

	if resolved.RequesterPlatform != "qq" {
		t.Fatalf("unexpected requester platform: %q", resolved.RequesterPlatform)
	}
	if resolved.RequesterUserID != "12345" {
		t.Fatalf("unexpected requester user id: %q", resolved.RequesterUserID)
	}
}

func TestLegacyCommandExecutionErrorReturnsReplySegments(t *testing.T) {
	status, message, data := legacyCommandExecutionError(onebot11.NewReplayError("请先上传 Suite"), "deck-event")

	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if message != "ok" {
		t.Fatalf("unexpected message: %q", message)
	}

	segments, ok := data.([]onebot11.Segment)
	if !ok {
		t.Fatalf("unexpected data type: %T", data)
	}
	if len(segments) != 1 || segments[0].Type != onebot11.TYPE_TEXT {
		t.Fatalf("unexpected segments: %+v", segments)
	}
	text, ok := segments[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected segment data: %#v", segments[0].Data)
	}
	if text.Text != "请先上传 Suite" {
		t.Fatalf("unexpected text: %q", text.Text)
	}
}
