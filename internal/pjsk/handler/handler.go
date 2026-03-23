package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"unicode"
)

var commandHandlerTree handlerTreeNode
var treeMutex = &sync.RWMutex{}
var maxDepth int

const DefaultPriority = 100

func Dispatch(ctx context.Context, event Event) (interface{}, error) {

	handlerContext := &HandlerContext{
		Context:     ctx,
		MessageType: event.MessageType,
		Message:     event.Message,
		Event:       event,
		MessageId:   event.MessageId,
		UserId:      event.UserId,
		SenderName:  event.SenderName,
		GroupId:     event.GroupId,
	}
	matched := MatchCommandHandler(event.Message)
	if matched.Handler == nil || matched.Handler.IsDisabled() {
		return nil, nil
	}
	handlerContext.ArgText = strings.TrimSpace(string(matched.ArgText))
	handlerContext.TriggerCmd = matched.Command
	return matched.Handler.Handle(handlerContext)
}

type CommandHandler interface {
	IsDisabled() bool
	GetCommands() []string
	GetPriority() int
	GetHelper() string
	Handle(Context) (interface{}, error)
}

type CommandHandlerBase struct {
	Disabled   bool
	Commands   []string
	Priority   int
	Helper     string
	handleFunc func(Context) (interface{}, error)
}

func (h *CommandHandlerBase) IsDisabled() bool {
	return h.Disabled
}

func (h *CommandHandlerBase) GetCommands() []string {
	return h.Commands
}
func (h *CommandHandlerBase) GetPriority() int {
	return h.Priority
}
func (h *CommandHandlerBase) GetHelper() string {
	return h.Helper
}
func (b *CommandHandlerBase) Handle(ctx Context) (interface{}, error) {
	if b.handleFunc != nil {
		return b.handleFunc(ctx)
	}
	cmdName := "未定义"
	if len(b.Commands) > 0 {
		cmdName = b.Commands[0]
	}
	return nil, fmt.Errorf("命令处理器 %s 没有处理方法", cmdName)
}

func RegisterCommandHandler(handler CommandHandler) {
	treeMutex.Lock()
	defer treeMutex.Unlock()
	for _, command := range handler.GetCommands() {
		commandHandlerTree.add(0, 0, []rune(command), handler)
	}
}

func MatchCommandHandler(message string) matchedHandler {
	treeMutex.RLock()
	defer treeMutex.RUnlock()
	messageRune := []rune(message)
	matched := commandHandlerTree.get(messageRune, 0, matchedHandler{})
	matched.ArgText = messageRune[matched.PrefixLength:]
	return matched
}

func IsCommandSeg(r rune) bool {
	switch r {
	case ' ':
		return true
	case '_':
		return true
	case '-':
		return true
	case '.':
		return true
	default:
		return false
	}
}

type handlerTreeNode struct {
	priority int
	depth    int
	command  string
	handler  CommandHandler
	children map[rune]*handlerTreeNode
}

func (t *handlerTreeNode) add(depth, index int, command []rune, handler CommandHandler) {
	var nextR rune
	for _, r := range command[index:] {
		index++
		if IsCommandSeg(r) {
			continue
		}
		nextR = unicode.ToLower(r)
		break
	}
	if nextR == 0 {
		handlerPriority := handler.GetPriority()
		if t.handler != nil && !t.handler.IsDisabled() {
			fmt.Fprintf(os.Stderr, "指令 %s 已被注册，已有的指令 %s 优先级：%d，待注册优先级：%d\n", string(command), string(t.command), t.priority, handlerPriority)
			if handlerPriority > 0 && (handlerPriority < t.priority || t.priority == 0) {
				log.Printf("待注册的指令处理器 %s 优先级更高，替换已有的处理器\n", string(command))
			} else {
				return
			}
		}
		t.priority = handlerPriority
		t.command = string(command)
		t.depth = depth
		if t.depth > maxDepth {
			maxDepth = t.depth
		}
		t.handler = handler
		return
	}
	if t.children == nil {
		t.children = make(map[rune]*handlerTreeNode)
	}
	child := t.children[nextR]
	if child == nil {
		child = &handlerTreeNode{}
	}
	child.add(depth+1, index, command, handler)
	t.children[nextR] = child
}

type matchedHandler struct {
	Command      string
	PrefixLength int
	ArgText      []rune
	Handler      CommandHandler
}

func (t *handlerTreeNode) get(command []rune, prefixLength int, macthed matchedHandler) matchedHandler {
	macthedPriority := 0
	if macthed.Handler != nil && !macthed.Handler.IsDisabled() {
		macthedPriority = macthed.Handler.GetPriority()
	}
	if t.handler != nil &&
		(macthedPriority == 0 ||
			(t.priority > 0 && t.priority <= macthedPriority)) {
		macthed.Command = t.command
		macthed.PrefixLength = prefixLength
		macthed.Handler = t.handler
	}
	var nextR rune
	for i, r := range command {
		prefixLength++
		if IsCommandSeg(r) {
			continue
		}
		nextR = unicode.ToLower(r)
		command = command[i+1:]
		break
	}
	if nextR == 0 {
		return macthed
	}
	if t.children == nil {
		return macthed
	}
	child := t.children[nextR]
	if child == nil {
		return macthed
	}
	return child.get(command, prefixLength, macthed)
}

func (t *handlerTreeNode) Json() []byte {
	if t == nil {
		return nil
	}
	jsonMap := make(map[string]interface{})
	jsonMap["command"] = t.command
	jsonMap["priority"] = t.priority
	jsonMap["depth"] = t.depth
	if len(t.children) == 0 {
		jsonMap["children"] = nil
	} else {
		childrenMap := make(map[string]json.RawMessage)
		for k, c := range t.children {
			childrenMap[string(k)] = c.Json()
		}
		jsonMap["children"] = childrenMap
	}
	result, err := json.Marshal(jsonMap)
	if err != nil {
		log.Println(err.Error())
	}
	return result
}

func PrintTree() {
	message := commandHandlerTree.Json()
	log.Println(string(message))
}
