package pjsk

import (
	"context"
	"fmt"

	botDB "haruki-cloud/database/bot"
	commandhandler "haruki-cloud/internal/handler"
)

// SeedCommandManifests synchronizes command_manifests from the registered
// handler-derived bot routes. Existing rows keep their
// current priority, but path-level protocol fields are refreshed on startup.
//
// This is called automatically by RegisterPJSKBotRoutes when a botDBClient is provided.
func SeedCommandManifests(ctx context.Context, client *botDB.Client) error {
	rows, err := client.CommandManifest.Query().All(ctx)
	if err != nil {
		return err
	}

	routes := commandhandler.ListBotRoutes()
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

func manifestKey(module, path string) string {
	return fmt.Sprintf("%s|%s", module, path)
}
