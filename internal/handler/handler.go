package handler

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"haruki-cloud/utils/logger"

	sonic "github.com/bytedance/sonic"
)

var commandRegistryLogger = logger.NewLoggerFromGlobal("CommandRegistry")

var commandHandlerTree handlerTreeNode
var commandLookupRegistry = map[string]*lookupEntry{}
var commandMatchRegistry []*lookupEntry
var treeMutex = &sync.RWMutex{}
var maxDepth int

const DefaultPriority = 100

type CommandHandler interface {
	IsDisabled() bool
	GetCommands() []string
	GetPath() string
	GetPriority() int
	GetHelper() string
}

type CommandHandlerBase struct {
	Disabled bool
	Commands []string
	Path     string
	Priority int
	Helper   string
}

func (b *CommandHandlerBase) IsDisabled() bool {
	return b.Disabled
}

func (b *CommandHandlerBase) GetCommands() []string {
	return b.Commands
}

func (b *CommandHandlerBase) GetPath() string {
	return b.Path
}

func (b *CommandHandlerBase) GetPriority() int {
	return b.Priority
}

func (b *CommandHandlerBase) GetHelper() string {
	return b.Helper
}

func RegisterCommandHandler(module string, handler CommandHandler) {
	treeMutex.Lock()
	defer treeMutex.Unlock()
	for _, command := range handler.GetCommands() {
		commandHandlerTree.add(0, 0, []rune(command), handler)
		registerCommandLookupLocked(command, handler)
	}
	registerBotRouteLocked(module, handler)
}

func MatchCommandHandler(message string) MatchedHandler {
	treeMutex.RLock()
	defer treeMutex.RUnlock()
	messageRune := []rune(message)
	matched := commandHandlerTree.get(messageRune, 0, MatchedHandler{})
	if matched.Handler != nil && !matched.Handler.IsDisabled() {
		if prefixLength, ok := MatchCommandPrefix(message, matched.Command); ok {
			matched.PrefixLength = prefixLength
			matched.ArgText = messageRune[prefixLength:]
			return matched
		}
	}
	matched = matchCommandHandlerStrictLocked(message)
	if matched.Handler == nil || matched.Handler.IsDisabled() {
		return MatchedHandler{}
	}
	matched.ArgText = messageRune[matched.PrefixLength:]
	return matched
}

func LookupCommandHandler(command string) (MatchedHandler, bool) {
	treeMutex.RLock()
	defer treeMutex.RUnlock()

	entry := commandLookupRegistry[normalizeCommandKey(command)]
	if entry == nil || entry.handler == nil || entry.handler.IsDisabled() {
		return MatchedHandler{}, false
	}
	return MatchedHandler{
		Command: entry.command,
		Handler: entry.handler,
	}, true
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

func ExtractCommandArgs(message, command string) (string, bool) {
	prefixLength, ok := MatchCommandPrefix(message, command)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(string([]rune(message)[prefixLength:])), true
}

func MatchCommandPrefix(message, command string) (int, bool) {
	messageRunes := []rune(message)
	commandRunes := []rune(command)

	messageIndex := 0
	commandIndex := 0

	for commandIndex < len(commandRunes) {
		skippedCommandSeg := false
		for commandIndex < len(commandRunes) && IsCommandSeg(commandRunes[commandIndex]) {
			skippedCommandSeg = true
			commandIndex++
		}
		if commandIndex >= len(commandRunes) {
			break
		}

		if skippedCommandSeg {
			for messageIndex < len(messageRunes) && IsCommandSeg(messageRunes[messageIndex]) {
				messageIndex++
			}
		} else if messageIndex < len(messageRunes) && IsCommandSeg(messageRunes[messageIndex]) {
			return 0, false
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
			commandRegistryLogger.Warn("command handler already registered",
				"command", string(command),
				"existing_command", t.command,
				"existing_priority", t.priority,
				"new_priority", handlerPriority,
			)
			if !shouldReplaceRegisteredHandler(t.handler, handler) {
				return
			}
			commandRegistryLogger.Info("command handler replaced", "command", string(command))
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

type lookupEntry struct {
	command string
	handler CommandHandler
}

type MatchedHandler struct {
	Command      string
	PrefixLength int
	ArgText      []rune
	Handler      CommandHandler
}

func registerCommandLookupLocked(command string, handler CommandHandler) {
	key := normalizeCommandKey(command)
	if key == "" {
		return
	}
	commandMatchRegistry = append(commandMatchRegistry, &lookupEntry{
		command: command,
		handler: handler,
	})

	existing := commandLookupRegistry[key]
	if existing != nil && existing.handler != nil && !existing.handler.IsDisabled() {
		if !shouldReplaceRegisteredHandler(existing.handler, handler) {
			return
		}
	}
	commandLookupRegistry[key] = &lookupEntry{
		command: command,
		handler: handler,
	}
}

func shouldReplaceRegisteredHandler(existing, incoming CommandHandler) bool {
	if existing == nil || existing.IsDisabled() {
		return true
	}

	existingPriority := existing.GetPriority()
	incomingPriority := incoming.GetPriority()
	return incomingPriority > 0 && (incomingPriority < existingPriority || existingPriority == 0)
}

func normalizeCommandKey(command string) string {
	var normalized []rune
	for _, r := range command {
		if IsCommandSeg(r) {
			continue
		}
		normalized = append(normalized, unicode.ToLower(r))
	}
	return string(normalized)
}

func matchCommandHandlerStrictLocked(message string) MatchedHandler {
	var best MatchedHandler
	for _, entry := range commandMatchRegistry {
		if entry == nil || entry.handler == nil || entry.handler.IsDisabled() {
			continue
		}
		prefixLength, ok := MatchCommandPrefix(message, entry.command)
		if !ok {
			continue
		}
		if !shouldPreferCommandMatch(entry, prefixLength, best) {
			continue
		}
		best.Command = entry.command
		best.PrefixLength = prefixLength
		best.Handler = entry.handler
	}
	return best
}

func shouldPreferCommandMatch(entry *lookupEntry, prefixLength int, current MatchedHandler) bool {
	if current.Handler == nil || current.Handler.IsDisabled() {
		return true
	}
	if prefixLength != current.PrefixLength {
		return prefixLength > current.PrefixLength
	}
	entryPriority := entry.handler.GetPriority()
	currentPriority := current.Handler.GetPriority()
	if entryPriority != currentPriority {
		if entryPriority == 0 {
			return false
		}
		if currentPriority == 0 {
			return true
		}
		return entryPriority < currentPriority
	}
	return len([]rune(entry.command)) > len([]rune(current.Command))
}

func (t *handlerTreeNode) get(command []rune, prefixLength int, matched MatchedHandler) MatchedHandler {
	matchedPriority := 0
	if matched.Handler != nil && !matched.Handler.IsDisabled() {
		matchedPriority = matched.Handler.GetPriority()
	}
	if t.handler != nil &&
		(matchedPriority == 0 ||
			(t.priority > 0 && t.priority <= matchedPriority)) {
		matched.Command = t.command
		matched.PrefixLength = prefixLength
		matched.Handler = t.handler
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
		return matched
	}
	if t.children == nil {
		return matched
	}
	child := t.children[nextR]
	if child == nil {
		return matched
	}
	return child.get(command, prefixLength, matched)
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
	result, err := sonic.Marshal(jsonMap)
	if err != nil {
		commandRegistryLogger.Error("command handler tree marshal failed", "error_type", fmt.Sprintf("%T", err))
	}
	return result
}

func PrintTree() {
	message := commandHandlerTree.Json()
	commandRegistryLogger.Debug("command handler tree built",
		"tree_bytes", len(message),
		"max_depth", maxDepth,
	)
}
