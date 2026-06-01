package pjsk

import (
	"context"
	"fmt"
	"slices"

	botDB "haruki-cloud/database/bot"
	commandhandler "haruki-cloud/internal/handler"
	pjskhandler "haruki-cloud/internal/pjsk/handler"
)

// SeedCommandManifests synchronizes command_manifests from the registered
// command manifest routes. Existing rows keep their
// current priority, but path-level protocol fields are refreshed on startup.
//
// This is called automatically by RegisterPJSKBotRoutes when a botDBClient is provided.
func SeedCommandManifests(ctx context.Context, client *botDB.Client) error {
	rows, err := client.CommandManifest.Query().All(ctx)
	if err != nil {
		return err
	}

	routes := commandManifestRoutes()
	existingByKey := make(map[string]*botDB.CommandManifest, len(rows))
	for _, row := range rows {
		existingByKey[manifestKey(row.CommandModule, row.CommandPath)] = row
	}

	bulk := make([]*botDB.CommandManifestCreate, 0, len(routes))
	for _, route := range routes {
		key := manifestKey(route.Module, route.Path)
		if row, ok := existingByKey[key]; ok {
			if _, err := client.CommandManifest.UpdateOneID(row.ID).
				SetCommandPrefixes(route.Commands).
				SetCommandMode(route.CommandMode).
				SetCommandAdditionalParams(route.AdditionalParams).
				Save(ctx); err != nil {
				return err
			}
			delete(existingByKey, key)
			continue
		}

		create := client.CommandManifest.Create().
			SetCommandPrefixes(route.Commands).
			SetCommandPriority(0).
			SetCommandMode(route.CommandMode).
			SetCommandModule(route.Module).
			SetCommandPath(route.Path).
			SetCommandAdditionalParams(route.AdditionalParams)
		bulk = append(bulk, create)
	}

	if len(bulk) == 0 {
		return nil
	}
	_, err = client.CommandManifest.CreateBulk(bulk...).Save(ctx)
	return err
}

func commandManifestRoutes() []commandhandler.BotRoute {
	routes := commandhandler.ListBotRoutes()
	routes = appendSpecialCommandManifestRoute(routes, birthdayMonitorManifestRoute())
	return appendSpecialCommandManifestRoute(routes, customProfileClientPolicyManifestRoute())
}

const birthdayMonitorClientPolicyScope = "birthday_monitor"

func birthdayMonitorManifestRoute() commandhandler.BotRoute {
	return commandhandler.BotRoute{
		Path:              birthdayMonitorCommandPath,
		Module:            pjskhandler.BotModulePJSK,
		Commands:          slices.Clone(birthdayMonitorManifestCommandPrefixes),
		CommandMode:       commandhandler.DefaultBotCommandMode,
		AdditionalParams:  slices.Clone(commandhandler.DefaultBotAdditionalParams),
		ClientPolicyScope: birthdayMonitorClientPolicyScope,
	}
}

const customProfileCommandPath = "profile/custom-profile-card"
const customProfileClientPolicyScope = "custom_profile"

func customProfileClientPolicyManifestRoute() commandhandler.BotRoute {
	return commandhandler.BotRoute{
		Path:              customProfileCommandPath,
		Module:            pjskhandler.BotModulePJSK,
		ClientPolicyScope: customProfileClientPolicyScope,
	}
}

func appendSpecialCommandManifestRoute(routes []commandhandler.BotRoute, special commandhandler.BotRoute) []commandhandler.BotRoute {
	specialKey := manifestKey(special.Module, special.Path)
	for index := range routes {
		if manifestKey(routes[index].Module, routes[index].Path) != specialKey {
			continue
		}
		routes[index].Commands = mergeCommandPrefixes(routes[index].Commands, special.Commands)
		if routes[index].CommandMode == "" {
			routes[index].CommandMode = special.CommandMode
		}
		if len(routes[index].AdditionalParams) == 0 {
			routes[index].AdditionalParams = slices.Clone(special.AdditionalParams)
		}
		if routes[index].ClientPolicyScope == "" {
			routes[index].ClientPolicyScope = special.ClientPolicyScope
		}
		return routes
	}
	return append(routes, special)
}

func commandManifestClientPolicyScopes() map[string]string {
	routes := commandManifestRoutes()
	scopes := make(map[string]string)
	for _, route := range routes {
		if route.ClientPolicyScope == "" {
			continue
		}
		scopes[manifestKey(route.Module, route.Path)] = route.ClientPolicyScope
	}
	return scopes
}

func mergeCommandPrefixes(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, commands := range [][]string{left, right} {
		for _, command := range commands {
			if _, ok := seen[command]; ok {
				continue
			}
			seen[command] = struct{}{}
			merged = append(merged, command)
		}
	}
	slices.Sort(merged)
	return merged
}

func manifestKey(module, path string) string {
	return fmt.Sprintf("%s|%s", module, path)
}
