package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"log"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
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
	Regions    []renderregion.Value
	PrefixArgs []string
	handleFunc func(SekaiHandlerContext) (interface{}, error)
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
func (s *SekaiHandlerContext) Flags() map[string]bool {
	return s.flags
}
func (s *SekaiHandlerContext) SetArgs(args string) {
	s.ArgText = args
}

func (skh *SekaiCommandHandler) Handle(ctx handler.Context) (interface{}, error) {
	if skh.handleFunc == nil {
		cmdName := "未定义"
		if len(skh.Commands) > 0 {
			cmdName = skh.Commands[0]
		}
		return nil, fmt.Errorf("Sekai 命令处理器 %s 没有处理方法", cmdName)
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
	for _, prefix := range skh.PrefixArgs {
		cmdPrefix := fmt.Sprintf("/%s", prefix)
		if strings.HasPrefix(triggerCmd, cmdPrefix) {
			prefixArg = prefix
			triggerCmd = strings.Replace(triggerCmd, cmdPrefix, "/", 1)
			break
		}
	}

	if cmdRegion.IsZero() && len(skh.Regions) > 0 {
		cmdRegion = skh.Regions[0]
	}

	args := ctx.GetArgs()
	uidArg := ""

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

var DefaultRegions = AllRegions

var registerOnce sync.Once

type sekaiHandlers struct{}

func EnsureCommandHandlersRegistered(nicknames map[string]int) {
	if nicknames != nil {
		SetNicknames(nicknames)
	}
	registerOnce.Do(registerSekaiCommandHandlers)
}

func RegisterSekaiCommandHandler() {
	EnsureCommandHandlersRegistered(nil)
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
			log.Printf("注册指令解析器：%s\n", methodName)
			results := methodVal.Call(nil)
			skHandler := results[0].Interface().(SekaiCommandHandler)

			if len(skHandler.Regions) == 0 {
				skHandler.Regions = DefaultRegions
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
							fmt.Fprintf(os.Stderr, "指令 %s 本身包含了区服前缀！", cmd)
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
