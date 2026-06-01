package handler

import (
	"slices"
	"strings"
)

const DefaultBotCommandMode = "POST"

var DefaultBotAdditionalParams []string

type BotRoute struct {
	Path              string
	Module            string
	Commands          []string
	CommandMode       string
	AdditionalParams  []string
	ClientPolicyScope string
}

type botRouteEntry struct {
	Path     string
	Module   string
	Commands map[string]struct{}
}

var botRouteRegistry = map[string]*botRouteEntry{}

func ListBotRoutes() []BotRoute {
	treeMutex.RLock()
	defer treeMutex.RUnlock()

	routes := make([]BotRoute, 0, len(botRouteRegistry))
	for _, route := range botRouteRegistry {
		commands := make([]string, 0, len(route.Commands))
		for command := range route.Commands {
			commands = append(commands, command)
		}
		slices.Sort(commands)
		routes = append(routes, BotRoute{
			Path:             route.Path,
			Module:           route.Module,
			Commands:         commands,
			CommandMode:      DefaultBotCommandMode,
			AdditionalParams: slices.Clone(DefaultBotAdditionalParams),
		})
	}
	slices.SortFunc(routes, func(a, b BotRoute) int {
		return strings.Compare(a.Path, b.Path)
	})
	return routes
}

func registerBotRouteLocked(module string, handler CommandHandler) {
	if handler == nil || handler.IsDisabled() {
		return
	}
	module = strings.TrimSpace(module)
	path := strings.TrimSpace(handler.GetPath())
	if module == "" || path == "" {
		return
	}

	route := botRouteRegistry[path]
	if route == nil {
		route = &botRouteEntry{
			Path:     path,
			Module:   module,
			Commands: map[string]struct{}{},
		}
		botRouteRegistry[path] = route
	}
	for _, command := range handler.GetCommands() {
		route.Commands[command] = struct{}{}
	}
}
