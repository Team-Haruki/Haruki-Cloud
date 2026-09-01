package handler

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	registryhandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/logger"
)

var sekaiRegistryLogger = logger.NewLoggerFromGlobal("PJSKCommandRegistry")

type HarrukiSekaiHandlerContext struct {
	PjskHandlerContext
	region             renderregion.Value // 区服
	explicitRegion     bool               // 是否由前缀或参数显式指定区服
	originalTriggerCmd string             // 原始触发命令，未去除区服前缀
	prefixArg          string             // 额外前缀
	uidArg             string             // UID参数
	flags              map[string]bool    // -verbose, -preview, -help 等开关
}

type HarukiSekaiCommandHandler struct {
	CommandHandlerBase
	Regions     []renderregion.Value
	PrefixArgs  []string
	ParseUIDArg *bool
	handleFunc  func(HarrukiSekaiHandlerContext) (*CommandRequest, error)
	executor    commandExecutor
}

func (s *HarrukiSekaiHandlerContext) Region() renderregion.Value {
	return s.region
}
func (s *HarrukiSekaiHandlerContext) HasExplicitRegion() bool {
	return s.explicitRegion
}
func (s *HarrukiSekaiHandlerContext) PrefixArg() string {
	return s.prefixArg
}
func (s *HarrukiSekaiHandlerContext) UIDArg() string {
	return s.uidArg
}
func (s *HarrukiSekaiHandlerContext) Flags() map[string]bool {
	return s.flags
}
func (s *HarrukiSekaiHandlerContext) SetArgs(args string) {
	s.ArgText = args
}

func (skh *HarukiSekaiCommandHandler) Handle(ctx Context) (*CommandRequest, error) {
	if skh.handleFunc == nil {
		return nil, skh.missingHandleFuncError()
	}

	originalTriggerCmd := ctx.GetTriggerCmd()
	input := skh.parseHandlerInput(ctx, originalTriggerCmd)
	skCtx := buildSekaiHandlerContext(ctx, input, originalTriggerCmd)
	if input.flags["is_help"] {
		resolved := makeCommandRequest(skCtx, parser.ModuleHelp, "help")
		skh.attachCommandMetadata(resolved, originalTriggerCmd)
		return resolved, nil
	}
	resolved, err := skh.handleFunc(skCtx)
	if err != nil || resolved == nil {
		return resolved, err
	}
	skh.attachCommandMetadata(resolved, originalTriggerCmd)
	if resolved.executor == nil {
		resolved.executor = skh.executor
	}
	return resolved, nil
}

func (skh *HarukiSekaiCommandHandler) missingHandleFuncError() error {
	commandName := "未定义"
	if len(skh.Commands) > 0 {
		commandName = skh.Commands[0]
	}
	return fmt.Errorf("sekai 命令处理器 %s 没有处理方法", commandName)
}

type sekaiHandlerInput struct {
	trigger        string
	args           string
	region         renderregion.Value
	explicitRegion bool
	prefixArg      string
	uidArg         string
	flags          map[string]bool
}

func (skh *HarukiSekaiCommandHandler) parseHandlerInput(ctx Context, trigger string) sekaiHandlerInput {
	input := resolveSekaiTrigger(skh.Regions, skh.PrefixArgs, trigger)
	input.args = ctx.GetArgs()
	extractor := parser.NewExtractor(nil)
	regionResult := extractor.ExtractRegion(input.args)
	input.args = regionResult.Remaining
	if normalized := renderregion.Normalize(regionResult.Value); !normalized.IsZero() {
		input.region = normalized
		input.explicitRegion = true
	}
	verboseResult := extractor.ExtractVerbose(input.args)
	input.flags["is_verbose"] = verboseResult.Value
	previewResult := extractor.ExtractPreview(verboseResult.Remaining)
	input.flags["is_preview"] = previewResult.Value
	helpResult := extractor.ExtractHelp(previewResult.Remaining)
	input.flags["is_help"] = helpResult.Value
	input.args = helpResult.Remaining
	if skh.shouldParseUIDArg() {
		input.uidArg, input.args = extractSekaiUIDArg(extractor, input.args, ctx.GetAtIds())
	}
	return input
}

func resolveSekaiTrigger(regions []renderregion.Value, prefixes []string, trigger string) sekaiHandlerInput {
	input := sekaiHandlerInput{trigger: trigger, flags: make(map[string]bool)}
	for _, region := range regions {
		regionPrefix := fmt.Sprintf("/%s", region)
		if strings.HasPrefix(input.trigger, regionPrefix) {
			input.region = region
			input.explicitRegion = true
			input.trigger = strings.Replace(input.trigger, regionPrefix, "/", 1)
			break
		}
	}
	if input.region.IsZero() && len(regions) > 0 {
		input.region = regions[0]
	}
	input.prefixArg = longestSekaiPrefix(prefixes, input.trigger)
	if input.prefixArg != "" || slices.Contains(prefixes, "") {
		input.trigger = strings.Replace(input.trigger, fmt.Sprintf("/%s", input.prefixArg), "/", 1)
	}
	return input
}

func longestSekaiPrefix(prefixes []string, trigger string) string {
	best := ""
	bestLength := -1
	for _, prefix := range prefixes {
		commandPrefix := fmt.Sprintf("/%s", prefix)
		if strings.HasPrefix(trigger, commandPrefix) && len(commandPrefix) > bestLength {
			best = prefix
			bestLength = len(commandPrefix)
		}
	}
	return best
}

func extractSekaiUIDArg(extractor *parser.Extractor, args string, atIDs []string) (string, string) {
	uidArg := ""
	result := extractor.ExtractUid(args)
	if result.Found {
		uidArg = result.Value
		args = result.Remaining
	}
	if len(atIDs) > 0 {
		uidArg = "@" + atIDs[0]
	}
	return uidArg, args
}

func buildSekaiHandlerContext(ctx Context, input sekaiHandlerInput, originalTrigger string) HarrukiSekaiHandlerContext {
	return HarrukiSekaiHandlerContext{
		Context:            ctx,
		Platform:           ctx.GetPlatform(),
		TriggerCmd:         input.trigger,
		ArgText:            input.args,
		MessageType:        ctx.GetMessageType(),
		Message:            ctx.GetMessage(),
		Event:              ctx.GetEvent(),
		MessageId:          ctx.GetMessageId(),
		UserId:             ctx.GetUserId(),
		SenderName:         ctx.GetSenderName(),
		GroupId:            ctx.GetGroupId(),
		AtIds:              ctx.GetAtIds(),
		region:             input.region,
		explicitRegion:     input.explicitRegion,
		originalTriggerCmd: originalTrigger,
		prefixArg:          input.prefixArg,
		uidArg:             input.uidArg,
		flags:              input.flags,
	}
}

func (skh *HarukiSekaiCommandHandler) attachCommandMetadata(resolved *CommandRequest, trigger string) {
	if resolved == nil {
		return
	}
	resolved.CommandPath = strings.TrimSpace(skh.Path)
	resolved.TriggerCommand = strings.TrimSpace(trigger)
	resolved.HelpText = strings.TrimSpace(skh.Helper)
}

func (skh *HarukiSekaiCommandHandler) shouldParseUIDArg() bool {
	if skh.ParseUIDArg == nil {
		return true
	}
	return *skh.ParseUIDArg
}

func commandBoolPtr(v bool) *bool {
	return &v
}

var registerOnce sync.Once

type sekaiHandlers struct{}

func EnsureCommandHandlersRegistered() {
	registerOnce.Do(registerSekaiCommandHandlers)
}

func RegisterSekaiCommandHandler() {
	EnsureCommandHandlersRegistered()
}

func registerSekaiCommandHandlers() {
	handlersVal := reflect.ValueOf(sekaiHandlers{})
	handlersTyp := handlersVal.Type()
	configTyp := reflect.TypeOf(HarukiSekaiCommandHandler{})
	for i := 0; i < handlersVal.NumMethod(); i++ {
		methodVal := handlersVal.Method(i)
		methodTyp := methodVal.Type()
		methodName := handlersTyp.Method(i).Name
		if !isSekaiHandlerFactory(methodTyp, configTyp) {
			continue
		}
		sekaiRegistryLogger.Info("command parser registered", "handler", methodName)
		skHandler := methodVal.Call(nil)[0].Interface().(HarukiSekaiCommandHandler)
		normalizeSekaiHandler(&skHandler, methodName)
		registryhandler.RegisterCommandHandler(BotModulePJSK, &skHandler)
	}
}

func isSekaiHandlerFactory(methodType, configType reflect.Type) bool {
	return methodType.NumIn() == 0 && methodType.NumOut() == 1 && methodType.Out(0) == configType
}

func normalizeSekaiHandler(handler *HarukiSekaiCommandHandler, methodName string) {
	if len(handler.Regions) == 0 {
		handler.Regions = AllRegions
	}
	if len(handler.PrefixArgs) == 0 {
		handler.PrefixArgs = []string{""}
	}
	handler.Commands = expandedSekaiCommands(handler.Commands, handler.Regions, handler.PrefixArgs)
	if handler.Priority == 0 {
		handler.Priority = DefaultPriority
	}
	if handler.executor == nil {
		panic(fmt.Sprintf("sekai command handler %s (%s) has no bound executor", methodName, handler.Path))
	}
}

func expandedSekaiCommands(commands []string, regions []renderregion.Value, prefixes []string) []string {
	allCommands := make(map[string]bool, len(commands)*len(regions)*len(prefixes))
	for _, prefix := range prefixes {
		for _, region := range regions {
			for _, command := range commands {
				regionText := string(region)
				if strings.HasPrefix(command, fmt.Sprintf("/%s%s", regionText, prefix)) {
					sekaiRegistryLogger.Warn("command already contains region prefix", "command", command)
				}
				allCommands[command] = true
				allCommands[strings.Replace(command, "/", fmt.Sprintf("/%s", prefix), 1)] = true
				allCommands[strings.Replace(command, "/", fmt.Sprintf("/%s%s", regionText, prefix), 1)] = true
			}
		}
	}
	expanded := make([]string, 0, len(allCommands))
	for command := range allCommands {
		expanded = append(expanded, command)
	}
	slices.Sort(expanded)
	return expanded
}
