package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unicode"
)

var commandHandlerTree handlerTreeNode
var treeMutex = &sync.RWMutex{}
var maxDepth int

const DefaultPriority = 100

func Dispatch(ctx context.Context, event Event) (any, error) {

	handlerContext, err := BuildContext(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("构建命令上下文失败: %w", err)
	}
	matched := MatchCommandHandler(handlerContext.GetArgs())
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
	GetPath() string
	GetPriority() int
	GetHelper() string
	Handle(Context) (any, error)
}

type CommandHandlerBase struct {
	Disabled   bool
	Commands   []string
	Path       string
	Priority   int
	Helper     string
	handleFunc func(Context) (any, error)
}

func (h *CommandHandlerBase) IsDisabled() bool {
	return h.Disabled
}

func (h *CommandHandlerBase) GetCommands() []string {
	return h.Commands
}
func (h *CommandHandlerBase) GetPath() string {
	return h.Path
}
func (h *CommandHandlerBase) GetPriority() int {
	return h.Priority
}
func (h *CommandHandlerBase) GetHelper() string {
	return h.Helper
}
func (b *CommandHandlerBase) Handle(ctx Context) (any, error) {
	if b.handleFunc != nil {
		return b.handleFunc(ctx)
	}
	cmdName := "未定义"
	if len(b.Commands) > 0 {
		cmdName = b.Commands[0]
	}
	return nil, fmt.Errorf("命令处理器 %s 没有处理方法", cmdName)
}

func RegisterCommandHandler(module string, handler CommandHandler) {
	treeMutex.Lock()
	defer treeMutex.Unlock()
	for _, command := range handler.GetCommands() {
		commandHandlerTree.add(0, 0, []rune(command), handler)
	}
	registerBotRouteLocked(module, handler)
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

// ExtractCommandArgs strips the matched command prefix from the original message
// using the same separator-insensitive matching rules as the handler trie.
func ExtractCommandArgs(message, command string) (string, bool) {
	prefixLength, ok := MatchCommandPrefix(message, command)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(string([]rune(message)[prefixLength:])), true
}

// MatchCommandPrefix reports whether command matches the start of message while
// ignoring command separators and ASCII case, returning the consumed rune count.
func MatchCommandPrefix(message, command string) (int, bool) {
	messageRunes := []rune(message)
	commandRunes := []rune(command)

	messageIndex := 0
	commandIndex := 0

	for commandIndex < len(commandRunes) {
		for commandIndex < len(commandRunes) && IsCommandSeg(commandRunes[commandIndex]) {
			commandIndex++
		}
		if commandIndex >= len(commandRunes) {
			break
		}

		for messageIndex < len(messageRunes) && IsCommandSeg(messageRunes[messageIndex]) {
			messageIndex++
		}
		if messageIndex >= len(messageRunes) {
			return 0, false
		}

		if unicode.ToLower(messageRunes[messageIndex]) != unicode.ToLower(commandRunes[commandIndex]) {
			return 0, false
		}

		messageIndex++
		commandIndex++
	}

	return messageIndex, true
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
			slog.Warn("指令已被注册", "command", string(command), "existing", string(t.command), "existing_priority", t.priority, "new_priority", handlerPriority)
			if handlerPriority > 0 && (handlerPriority < t.priority || t.priority == 0) {
				slog.Info("待注册的指令处理器优先级更高，替换已有的处理器", "command", string(command))
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
	jsonMap := make(map[string]any)
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
		slog.Error("failed to marshal handler tree", "error", err)
	}
	return result
}

func PrintTree() {
	message := commandHandlerTree.Json()
	slog.Debug("handler tree", "tree", string(message))
}
