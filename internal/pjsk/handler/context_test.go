package handler

import (
	"context"
	"reflect"
	"testing"

	zeromessage "github.com/wdvxdr1123/ZeroBot/message"
)

func TestBuildContextPreservesEventFieldsAndExtractsAtIDs(t *testing.T) {
	event := Event{
		Platform:    "qq",
		MessageType: MessageTypeGroup,
		Message: []zeromessage.Segment{
			{Type: "text", Data: map[string]string{"text": "/sk "}},
			{Type: "at", Data: map[string]string{"qq": "12345"}},
			{Type: "text", Data: map[string]string{"text": " 20"}},
		},
		MessageId:  "mid-1",
		UserId:     "u-1",
		SenderName: "tester",
		GroupId:    "g-1",
	}

	ctx, err := BuildContext(context.Background(), event, "/sk")
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if ctx.GetTriggerCmd() != "/sk" {
		t.Fatalf("GetTriggerCmd() = %q", ctx.GetTriggerCmd())
	}
	if ctx.GetPlatform() != event.Platform {
		t.Fatalf("GetPlatform() = %q", ctx.GetPlatform())
	}
	if ctx.GetMessageType() != event.MessageType {
		t.Fatalf("GetMessageType() = %q", ctx.GetMessageType())
	}
	if ctx.GetMessageId() != event.MessageId {
		t.Fatalf("GetMessageId() = %q", ctx.GetMessageId())
	}
	if ctx.GetUserId() != event.UserId {
		t.Fatalf("GetUserId() = %q", ctx.GetUserId())
	}
	if ctx.GetSenderName() != event.SenderName {
		t.Fatalf("GetSenderName() = %q", ctx.GetSenderName())
	}
	if ctx.GetGroupId() != event.GroupId {
		t.Fatalf("GetGroupId() = %q", ctx.GetGroupId())
	}
	if !reflect.DeepEqual(ctx.GetMessage(), event.Message) {
		t.Fatalf("GetMessage() = %#v", ctx.GetMessage())
	}
	if !reflect.DeepEqual(ctx.GetAtIds(), []string{"12345"}) {
		t.Fatalf("GetAtIds() = %#v", ctx.GetAtIds())
	}
	if ctx.GetArgs() != "/sk  20" {
		t.Fatalf("GetArgs() = %q", ctx.GetArgs())
	}
}
