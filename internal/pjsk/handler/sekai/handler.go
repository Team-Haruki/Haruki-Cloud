package sekai

import (
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type SekaiHandlerContext struct {
	handler.HandlerContext
	region             renderregion.Value // 区服
	explicitRegion     bool               // 是否由前缀或参数显式指定区服
	originalTriggerCmd string             // 原始触发命令，未去除区服前缀
	prefixArg          string             // 额外前缀
	uidArg             string             // UID参数
	flags              map[string]bool    // -verbose, -preview, -help 等开关
}

type SekaiCommandHandler struct {
	handler.CommandHandlerBase
	Regions     []renderregion.Value
	PrefixArgs  []string
	ParseUIDArg *bool
	handleFunc  func(SekaiHandlerContext) (any, error)
}

func (s *SekaiHandlerContext) Region() renderregion.Value {
	return s.region
}
func (s *SekaiHandlerContext) HasExplicitRegion() bool {
	return s.explicitRegion
}
func (s *SekaiHandlerContext) PrefixArg() string {
	return s.prefixArg
}
func (s *SekaiHandlerContext) UIDArg() string {
	return s.uidArg
}
func (s *SekaiHandlerContext) Flags() map[string]bool {
	return s.flags
}
func (s *SekaiHandlerContext) SetArgs(args string) {
	s.ArgText = args
}

func (skh *SekaiCommandHandler) Handle(ctx handler.Context) (any, error) {
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

	skCtx := SekaiHandlerContext{
		HandlerContext: handler.HandlerContext{
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
		},
		region:             cmdRegion,
		explicitRegion:     explicitRegion,
		originalTriggerCmd: originalTriggerCmd,
		prefixArg:          prefixArg,
		uidArg:             uidArg,
		flags:              flags,
	}
	return skh.handleFunc(skCtx)
}

func (skh *SekaiCommandHandler) shouldParseUIDArg() bool {
	if skh.ParseUIDArg == nil {
		return true
	}
	return *skh.ParseUIDArg
}

func boolPtr(v bool) *bool {
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
	configTyp := reflect.TypeOf(SekaiCommandHandler{})
	for i := 0; i < handlersVal.NumMethod(); i++ {
		methodVal := handlersVal.Method(i)
		methodTyp := methodVal.Type()
		methodName := handlersTyp.Method(i).Name
		if methodTyp.NumIn() == 0 &&
			methodTyp.NumOut() == 1 &&
			methodTyp.Out(0) == configTyp {
			slog.Info("注册指令解析器", "handler", methodName)
			results := methodVal.Call(nil)
			skHandler := results[0].Interface().(SekaiCommandHandler)

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
							slog.Warn("指令本身包含了区服前缀", "cmd", cmd)
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
				skHandler.Priority = handler.DefaultPriority
			}
			handler.RegisterCommandHandler(handler.BotModulePJSK, &skHandler)
		}
	}
}
