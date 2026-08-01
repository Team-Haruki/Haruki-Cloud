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
		cmdName := "未定义"
		if len(skh.Commands) > 0 {
			cmdName = skh.Commands[0]
		}
		return nil, fmt.Errorf("sekai 命令处理器 %s 没有处理方法", cmdName)
	}

	var cmdRegion renderregion.Value
	explicitRegion := false
	originalTriggerCmd := ctx.GetTriggerCmd()
	triggerCmd := originalTriggerCmd
	for _, region := range skh.Regions {
		cmdRegionPrefix := fmt.Sprintf("/%s", string(region))
		if strings.HasPrefix(triggerCmd, cmdRegionPrefix) {
			cmdRegion = region
			explicitRegion = true
			triggerCmd = strings.Replace(triggerCmd, cmdRegionPrefix, "/", 1)
			break
		}
	}

	prefixArg := ""
	bestPrefixLen := -1
	for _, prefix := range skh.PrefixArgs {
		cmdPrefix := fmt.Sprintf("/%s", prefix)
		if strings.HasPrefix(triggerCmd, cmdPrefix) {
			if len(cmdPrefix) <= bestPrefixLen {
				continue
			}
			prefixArg = prefix
			bestPrefixLen = len(cmdPrefix)
		}
	}
	if bestPrefixLen >= 0 {
		triggerCmd = strings.Replace(triggerCmd, fmt.Sprintf("/%s", prefixArg), "/", 1)
	}

	if cmdRegion.IsZero() && len(skh.Regions) > 0 {
		cmdRegion = skh.Regions[0]
	}

	args := ctx.GetArgs()

	ext := parser.NewExtractor(nil)
	flags := make(map[string]bool)

	regRes := ext.ExtractRegion(args)
	if regRes.Value != "" {
		normalized := renderregion.Normalize(regRes.Value)
		if !normalized.IsZero() {
			cmdRegion = normalized
			explicitRegion = true
		}
	}
	args = regRes.Remaining

	verbRes := ext.ExtractVerbose(args)
	flags["is_verbose"] = verbRes.Value
	args = verbRes.Remaining

	preRes := ext.ExtractPreview(args)
	flags["is_preview"] = preRes.Value
	args = preRes.Remaining

	helpRes := ext.ExtractHelp(args)
	flags["is_help"] = helpRes.Value
	args = helpRes.Remaining
	uidArg := ""

	if skh.shouldParseUIDArg() {
		uidRes := ext.ExtractUid(args)
		if uidRes.Found {
			uidArg = uidRes.Value
			args = uidRes.Remaining
		}
		if atIDs := ctx.GetAtIds(); len(atIDs) > 0 {
			uidArg = "@" + atIDs[0]
		}
	}

	skCtx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{
			Context:     ctx,
			Platform:    ctx.GetPlatform(),
			TriggerCmd:  triggerCmd,
			ArgText:     args,
			MessageType: ctx.GetMessageType(),
			Message:     ctx.GetMessage(),
			Event:       ctx.GetEvent(),
			MessageId:   ctx.GetMessageId(),
			UserId:      ctx.GetUserId(),
			SenderName:  ctx.GetSenderName(),
			GroupId:     ctx.GetGroupId(),
			AtIds:       ctx.GetAtIds(),
		},
		region:             cmdRegion,
		explicitRegion:     explicitRegion,
		originalTriggerCmd: originalTriggerCmd,
		prefixArg:          prefixArg,
		uidArg:             uidArg,
		flags:              flags,
	}
	if flags["is_help"] {
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
		if methodTyp.NumIn() == 0 &&
			methodTyp.NumOut() == 1 &&
			methodTyp.Out(0) == configTyp {
			sekaiRegistryLogger.Info("command parser registered", "handler", methodName)
			results := methodVal.Call(nil)
			skHandler := results[0].Interface().(HarukiSekaiCommandHandler)

			if len(skHandler.Regions) == 0 {
				skHandler.Regions = AllRegions
			}
			if len(skHandler.PrefixArgs) == 0 {
				skHandler.PrefixArgs = []string{""}
			}
			allRegionCommands := make(map[string]bool, len(skHandler.Commands)*len(skHandler.Regions)*len(skHandler.PrefixArgs))
			for _, prefix := range skHandler.PrefixArgs {
				for _, region := range skHandler.Regions {
					for _, cmd := range skHandler.Commands {
						regionStr := string(region)
						if strings.HasPrefix(cmd, fmt.Sprintf("/%s%s", regionStr, prefix)) {
							sekaiRegistryLogger.Warn("command already contains region prefix", "command", cmd)
						}
						allRegionCommands[cmd] = true
						allRegionCommands[strings.Replace(cmd, "/", fmt.Sprintf("/%s", prefix), 1)] = true
						allRegionCommands[strings.Replace(cmd, "/", fmt.Sprintf("/%s%s", regionStr, prefix), 1)] = true
					}
				}
			}
			skHandler.Commands = make([]string, 0, len(allRegionCommands))
			for cmd := range allRegionCommands {
				skHandler.Commands = append(skHandler.Commands, cmd)
			}
			slices.Sort(skHandler.Commands)
			if skHandler.Priority == 0 {
				skHandler.Priority = DefaultPriority
			}
			if skHandler.executor == nil {
				panic(fmt.Sprintf("sekai command handler %s (%s) has no bound executor", methodName, skHandler.Path))
			}
			registryhandler.RegisterCommandHandler(BotModulePJSK, &skHandler)
		}
	}
}
