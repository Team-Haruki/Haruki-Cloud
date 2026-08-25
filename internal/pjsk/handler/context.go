package handler

import (
	"context"
	"fmt"
	"haruki-cloud/internal/onebot11"
	"regexp"
	"strings"
)

type MessageType string

const (
	MessageTypePrivate MessageType = "private"
	MessageTypeGroup   MessageType = "group"
)

type Event struct {
	Platform    string
	MessageType MessageType
	Message     onebot11.Message
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
	GetMessage() onebot11.Message
	GetMessageId() string
	GetUserId() string
	GetSenderName() string
	GetGroupId() string
	GetAtIds() []string
}

func BuildContext(ctx context.Context, event Event) (*PjskHandlerContext, error) {
	handleCtx := PjskHandlerContext{
		Context:     ctx,
		Event:       event,
		Platform:    event.Platform,
		MessageType: event.MessageType,
		Message:     event.Message,
		MessageId:   event.MessageId,
		UserId:      event.UserId,
		SenderName:  event.SenderName,
		GroupId:     event.GroupId,

		AtIds:   extractAtIds(event.Message),
		ArgText: extractText(event.Message)}
	return &handleCtx, nil
}

type PjskHandlerContext struct {
	context.Context
	Platform    string
	TriggerCmd  string
	ArgText     string
	MessageType MessageType
	Event       Event
	Message     onebot11.Message
	MessageId   string
	UserId      string
	SenderName  string
	GroupId     string
	AtIds       []string
}

func (h *PjskHandlerContext) GetTriggerCmd() string {
	return h.TriggerCmd
}
func (h *PjskHandlerContext) GetArgs() string {
	return h.ArgText
}
func (h *PjskHandlerContext) GetPlatform() string {
	return h.Platform
}
func (h *PjskHandlerContext) GetMessageType() MessageType {
	return h.MessageType
}
func (h *PjskHandlerContext) GetEvent() Event {
	return h.Event
}
func (h *PjskHandlerContext) GetMessage() onebot11.Message {
	return h.Message
}
func (h *PjskHandlerContext) GetMessageId() string {
	return h.MessageId
}
func (h *PjskHandlerContext) GetUserId() string {
	return h.UserId
}
func (h *PjskHandlerContext) GetSenderName() string {
	return h.SenderName
}
func (h *PjskHandlerContext) GetGroupId() string {
	return h.GroupId
}
func (h *PjskHandlerContext) GetAtIds() []string {
	return h.AtIds
}

func extractAtIds(segments onebot11.Message) []string {
	var atIds []string
	for _, seg := range segments {
		if seg.Type != onebot11.TypeAt {
			continue
		}

		if atData, ok := seg.Data.(onebot11.AtData); ok && strings.TrimSpace(atData.QQ) != "" {
			atIds = append(atIds, strings.TrimSpace(atData.QQ))
			continue
		}

		if qq, ok := extractSegmentDataField(seg.Data, onebot11.KeyQQ); ok && strings.TrimSpace(qq) != "" {
			atIds = append(atIds, strings.TrimSpace(qq))
		}
	}
	return atIds
}

func extractText(segments onebot11.Message) string {
	var text string
	for _, seg := range segments {
		if seg.Type != onebot11.TypeText {
			continue
		}

		if textData, ok := seg.Data.(onebot11.TextData); ok {
			text += stripInlineCQTags(textData.Text)
			continue
		}

		if raw, ok := extractSegmentDataField(seg.Data, onebot11.KeyText); ok {
			text += stripInlineCQTags(raw)
		}
	}
	return text
}

var inlineCQPattern = regexp.MustCompile(`(?i)\[cq:[^\]]+\]`)

func stripInlineCQTags(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return inlineCQPattern.ReplaceAllString(text, " ")
}

func extractSegmentDataField(data any, key string) (string, bool) {
	switch d := data.(type) {
	case map[string]string:
		v, ok := d[key]
		return v, ok
	case map[string]any:
		if v, ok := d[key]; ok {
			return fmt.Sprint(v), true
		}
	case map[any]any:
		if v, ok := d[key]; ok {
			return fmt.Sprint(v), true
		}
		for k, v := range d {
			if ks, ok := k.(string); ok && ks == key {
				return fmt.Sprint(v), true
			}
		}
	}
	return "", false
}
