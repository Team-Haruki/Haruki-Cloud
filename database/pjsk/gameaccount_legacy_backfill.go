package pjsk

import (
	"context"
	"fmt"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
)

const (
	legacyBindingIdentityProbeSQL = `SELECT server, user_id FROM user_bindings LIMIT 1`
	legacyProfileBackgroundProbe  = `SELECT server, user_id, bg FROM profile_backgrounds LIMIT 1`

	backfillGameAccountsFromBindingsSQL = `
INSERT INTO game_accounts (server, user_id)
SELECT DISTINCT lower(trim(server)), trim(user_id)
FROM user_bindings
WHERE trim(COALESCE(server, '')) <> '' AND trim(COALESCE(user_id, '')) <> ''
ON CONFLICT (server, user_id) DO NOTHING
`

	backfillGameAccountsFromProfileBackgroundsSQL = `
INSERT INTO game_accounts (server, user_id, bg)
SELECT lower(trim(server)), trim(user_id), bg
FROM profile_backgrounds
WHERE trim(COALESCE(server, '')) <> '' AND trim(COALESCE(user_id, '')) <> ''
ON CONFLICT (server, user_id) DO UPDATE
SET bg = excluded.bg
WHERE excluded.bg IS NOT NULL
`

	backfillBindingGameAccountIDsSQL = `
UPDATE user_bindings
SET game_account_id = (
	SELECT ga.id
	FROM game_accounts AS ga
	WHERE ga.server = lower(trim(user_bindings.server))
	  AND ga.user_id = trim(user_bindings.user_id)
	LIMIT 1
)
WHERE game_account_id IS NULL
  AND trim(COALESCE(server, '')) <> ''
  AND trim(COALESCE(user_id, '')) <> ''
`
)

// BackfillLegacyGameAccounts migrates legacy PJSK binding/background data into
// the new game_accounts table and wires user_bindings.game_account_id to it.
//
// It is safe to call repeatedly. On fresh schemas that no longer contain the
// legacy table/columns, it becomes a no-op.
func (c *Client) BackfillLegacyGameAccounts(ctx context.Context) error {
	if c == nil {
		return nil
	}

	hasLegacyBindingIdentity, err := c.legacyQueryAvailable(ctx, legacyBindingIdentityProbeSQL)
	if err != nil {
		return fmt.Errorf("pjsk: probe legacy binding identity columns: %w", err)
	}
	hasLegacyProfileBackgrounds, err := c.legacyQueryAvailable(ctx, legacyProfileBackgroundProbe)
	if err != nil {
		return fmt.Errorf("pjsk: probe legacy profile backgrounds: %w", err)
	}

	if hasLegacyBindingIdentity {
		if err := c.execRaw(ctx, backfillGameAccountsFromBindingsSQL); err != nil {
			return fmt.Errorf("pjsk: backfill game accounts from legacy bindings: %w", err)
		}
		if err := c.execRaw(ctx, backfillBindingGameAccountIDsSQL); err != nil {
			return fmt.Errorf("pjsk: backfill binding game_account_id: %w", err)
		}
	}

	if hasLegacyProfileBackgrounds {
		if err := c.execRaw(ctx, backfillGameAccountsFromProfileBackgroundsSQL); err != nil {
			return fmt.Errorf("pjsk: backfill game account backgrounds: %w", err)
		}
	}

	return nil
}

func (c *Client) legacyQueryAvailable(ctx context.Context, query string) (bool, error) {
	var rows entsql.Rows
	err := c.driver.Query(ctx, query, []any{}, &rows)
	if err != nil {
		if isMissingLegacySchemaError(err) {
			return false, nil
		}
		return false, err
	}
	defer rows.Close()
	return true, nil
}

func (c *Client) execRaw(ctx context.Context, query string) error {
	var result entsql.Result
	return c.driver.Exec(ctx, query, []any{}, &result)
}

func isMissingLegacySchemaError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "no such table") ||
		strings.Contains(text, "does not exist") ||
		strings.Contains(text, "no such column") ||
		strings.Contains(text, "column does not exist") ||
		strings.Contains(text, "has no column named") ||
		strings.Contains(text, "unknown column")
}
