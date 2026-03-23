package handler

import "context"

type MessageType string

const (
	MessageTypePrivate MessageType = "private"
	MessageTypeGroup   MessageType = "group"
)

type Event struct {
	MessageType MessageType
	Message     string
	MessageId   string
	UserId      string
	SenderName  string
	GroupId     string
}

type Context interface {
	context.Context
	GetTriggerCmd() string
	GetArgs() string
	GetMessageType() MessageType
	GetMessage() string
	GetEvent() Event
	GetMessageId() string
	GetUserId() string
	GetSenderName() string
	GetGroupId() string
}

type HandlerContext struct {
	context.Context
	TriggerCmd  string
	ArgText     string
	MessageType MessageType
	Message     string
	Event       Event
	MessageId   string
	UserId      string
	SenderName  string
	GroupId     string
}

func (h *HandlerContext) GetTriggerCmd() string {
	return h.TriggerCmd
}
func (h *HandlerContext) GetArgs() string {
	return h.ArgText
}
func (h *HandlerContext) GetMessageType() MessageType {
	return h.MessageType
}
func (h *HandlerContext) GetMessage() string {
	return h.Message
}
func (h *HandlerContext) GetEvent() Event {
	return h.Event
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
