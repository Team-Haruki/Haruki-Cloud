package handler

import (
	"context"
	"fmt"
	"haruki-cloud/api/bot/onebot11"
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
	Message     onebot11.Message
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
func (h *HandlerContext) GetMessage() onebot11.Message {
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

func extractAtIds(segments onebot11.Message) []string {
	var atIds []string
	for _, seg := range segments {
		if seg.Type != onebot11.TYPE_AT {
			continue
		}

		if atData, ok := seg.Data.(onebot11.AtData); ok && strings.TrimSpace(atData.QQ) != "" {
			atIds = append(atIds, strings.TrimSpace(atData.QQ))
			continue
		}

		if qq, ok := extractSegmentDataField(seg.Data, onebot11.KEY_QQ); ok && strings.TrimSpace(qq) != "" {
			atIds = append(atIds, strings.TrimSpace(qq))
		}
	}
	return atIds
}

func extractText(segments onebot11.Message) string {
	var text string
	for _, seg := range segments {
		if seg.Type != onebot11.TYPE_TEXT {
			continue
		}

		if textData, ok := seg.Data.(onebot11.TextData); ok {
			text += stripInlineCQTags(textData.Text)
			continue
		}

		if raw, ok := extractSegmentDataField(seg.Data, onebot11.KEY_TEXT); ok {
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
	case map[string]interface{}:
		if v, ok := d[key]; ok {
			return fmt.Sprint(v), true
		}
	case map[interface{}]interface{}:
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
