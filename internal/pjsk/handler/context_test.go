package handler

import (
	"context"
	"haruki-cloud/internal/pjsk/onebot11"
	"reflect"
	"testing"
)

func TestBuildContextPreservesEventFieldsAndExtractsAtIDs(t *testing.T) {
	event := Event{
		Platform:    "qq",
		MessageType: MessageTypeGroup,
		Message: onebot11.Message{
			{Type: "text", Data: map[string]string{"text": "/sk "}},
			{Type: "at", Data: map[string]string{"qq": "12345"}},
			{Type: "text", Data: map[string]string{"text": " 20"}},
		},
		MessageId:  "mid-1",
		UserId:     "u-1",
		SenderName: "tester",
		GroupId:    "g-1",
	}

	ctx, err := BuildContext(context.Background(), event)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
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

func TestBuildContextHandlesTypedSegments(t *testing.T) {
	event := Event{
		Platform:    "qq",
		MessageType: MessageTypeGroup,
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/cnsk "}},
			{Type: "at", Data: onebot11.AtData{QQ: "67890"}},
			{Type: "text", Data: onebot11.TextData{Text: " 100"}},
		},
	}

	ctx, err := BuildContext(context.Background(), event)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if !reflect.DeepEqual(ctx.GetAtIds(), []string{"67890"}) {
		t.Fatalf("GetAtIds() = %#v", ctx.GetAtIds())
	}
	if ctx.GetArgs() != "/cnsk  100" {
		t.Fatalf("GetArgs() = %q", ctx.GetArgs())
	}
}

func TestBuildContextExtractsAtFromInterfaceMap(t *testing.T) {
	event := Event{
		Platform:    "qq",
		MessageType: MessageTypeGroup,
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/sk "}},
			{Type: "at", Data: map[string]any{"qq": 1234567890}},
		},
	}

	ctx, err := BuildContext(context.Background(), event)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if !reflect.DeepEqual(ctx.GetAtIds(), []string{"1234567890"}) {
		t.Fatalf("GetAtIds() = %#v", ctx.GetAtIds())
	}
	if ctx.GetArgs() != "/sk " {
		t.Fatalf("GetArgs() = %q", ctx.GetArgs())
	}
}

func TestBuildContextStripsInlineCQTagsFromText(t *testing.T) {
	event := Event{
		Platform:    "qq",
		MessageType: MessageTypeGroup,
		Message: onebot11.Message{
			{Type: "text", Data: onebot11.TextData{Text: "/sk[CQ:at,qq=1000000001]"}},
			{Type: "at", Data: onebot11.AtData{QQ: "1000000001"}},
		},
	}

	ctx, err := BuildContext(context.Background(), event)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if ctx.GetArgs() != "/sk " {
		t.Fatalf("GetArgs() = %q", ctx.GetArgs())
	}
	if !reflect.DeepEqual(ctx.GetAtIds(), []string{"1000000001"}) {
		t.Fatalf("GetAtIds() = %#v", ctx.GetAtIds())
	}
}
