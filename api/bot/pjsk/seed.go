package pjsk

import (
	"context"

	botDB "haruki-cloud/database/bot"
)

// SeedCommandManifests populates the command_manifests table from the built-in
// botModeTable if the table is currently empty. Existing rows are left untouched,
// so manually adjusted priorities and prefixes are preserved across restarts.
//
// This is called automatically by RegisterPJSKBotRoutes when a botDBClient is provided.
func SeedCommandManifests(ctx context.Context, client *botDB.Client) error {
	count, err := client.CommandManifest.Query().Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	bulk := make([]*botDB.CommandManifestCreate, 0, len(botModeTable))
	for _, entry := range botModeTable {
		create := client.CommandManifest.Create().
			SetCommandPrefixes(entry.prefixes).
			SetCommandPriority(0).
			SetCommandMode("GET,POST").
			SetCommandModule(moduleNameStr(entry.module)).
			SetCommandPath(entry.path).
			SetCommandAdditionalParams([]string{})
		bulk = append(bulk, create)
	}

	_, err = client.CommandManifest.CreateBulk(bulk...).Save(ctx)
	return err
}
