package cachefill

import (
	"errors"
	"strings"
)

// IsMissingTable reports whether err is a database's "table does not exist"
// error. Master tables introduced by a newer game client exist in the
// database only after the ingest side ships them. Such a fill still fails
// (it is not cached and backs off like any failure, so the table is picked
// up once it exists) but is logged at debug level instead of as an outage.
func IsMissingTable(err error) bool {
	if err == nil {
		return false
	}
	var coded interface{ SQLState() string }
	if errors.As(err, &coded) && coded.SQLState() == "42P01" {
		return true
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "42p01"):
		// PostgreSQL undefined_table (lib/pq and pgx both print the code).
		return true
	case strings.Contains(msg, "relation \"") && strings.Contains(msg, " does not exist") && !strings.Contains(msg, "column "):
		return true
	case strings.Contains(msg, "error 1146"):
		// MySQL ER_NO_SUCH_TABLE.
		return true
	case strings.Contains(msg, "no such table"):
		// SQLite.
		return true
	}
	return false
}
