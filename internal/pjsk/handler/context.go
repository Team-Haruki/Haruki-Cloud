package handler

import (
	"context"

	zeromessage "github.com/wdvxdr1123/ZeroBot/message"
)

type MessageType string

const (
	MessageTypePrivate MessageType = "private"
	MessageTypeGroup   MessageType = "group"
)

type Event struct {
	Platform    string
	MessageType MessageType
	Message     []zeromessage.Segment
	MessageId   string
	UserId      string
	SenderName  string
	GroupId     string
}

type Context interface {
	context.Context
	GetTriggerCmd() string
	GetArgs() string
	GetPlatform() string
	GetMessageType() MessageType
	GetEvent() Event
	GetMessage() []zeromessage.Segment
	GetMessageId() string
	GetUserId() string
	GetSenderName() string
	GetGroupId() string
	GetAtIds() []string
}

func BuildContext(ctx context.Context, event Event) (*HandlerContext, error) {
	handleCtx := HandlerContext{
		Context:     ctx,
		Event:       event,
		Platform:    event.Platform,
		MessageType: event.MessageType,
		Message:     event.Message,
		MessageId:   event.MessageId,
		UserId:      event.UserId,
		SenderName:  event.SenderName,
		GroupId:     event.GroupId,
	}

	handleCtx.AtIds = extractAtIds(event.Message)
	handleCtx.ArgText = extractText(event.Message)
	return &handleCtx, nil
}

type HandlerContext struct {
	context.Context
	Platform    string
	TriggerCmd  string
	ArgText     string
	MessageType MessageType
	Event       Event
	Message     []zeromessage.Segment
	MessageId   string
	UserId      string
	SenderName  string
	GroupId     string
	AtIds       []string
}

func (h *HandlerContext) GetTriggerCmd() string {
	return h.TriggerCmd
}
func (h *HandlerContext) GetArgs() string {
	return h.ArgText
}
func (h *HandlerContext) GetPlatform() string {
	return h.Platform
}
func (h *HandlerContext) GetMessageType() MessageType {
	return h.MessageType
}
func (h *HandlerContext) GetEvent() Event {
	return h.Event
}
func (h *HandlerContext) GetMessage() []zeromessage.Segment {
	return h.Message
}
func (h *HandlerContext) GetMessageId() string {
	return h.MessageId
}
func (h *HandlerContext) GetUserId() string {
	return h.UserId
}
func (h *HandlerContext) GetSenderName() string {
	return h.SenderName
}
func (h *HandlerContext) GetGroupId() string {
	return h.GroupId
}
func (h *HandlerContext) GetAtIds() []string {
	return h.AtIds
}

func extractAtIds(segments []zeromessage.Segment) []string {
	var atIds []string
	for _, seg := range segments {
		if seg.Type == "at" {
			if id, ok := seg.Data["qq"]; ok && id != "" {
				atIds = append(atIds, id)
				continue
			}
		}
	}
	return atIds
}

func extractText(segments []zeromessage.Segment) string {
	var text string
	for _, seg := range segments {
		if seg.Type == "text" {
			if content, ok := seg.Data["text"]; ok {
				text += content
			}
		}
	}
	return text
}
