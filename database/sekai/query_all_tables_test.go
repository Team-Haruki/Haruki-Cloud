package sekai_test

import (
	"context"
	"os"
	"reflect"
	"sort"
	"testing"

	entsekai "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/migrate"
	"haruki-cloud/internal/testutil"

	_ "github.com/lib/pq"
)

const defaultSekaiDSN = "host=localhost port=5432 user=haruki password=sekai dbname=master_data sslmode=disable"

func TestSekaiQueryAllTables(t *testing.T) {
	if os.Getenv("HARUKI_RUN_SEKAI_DB_TEST") != "1" {
		t.Skip("set HARUKI_RUN_SEKAI_DB_TEST=1 to run sekai DB integration test")
	}

	dsn := os.Getenv("HARUKI_SEKAI_DB_DSN")
	if dsn == "" {
		dsn = defaultSekaiDSN
	}

	client, err := entsekai.Open("postgres", dsn)
	testutil.Require(t, !(err != nil), "open sekai client: %v", err)

	defer func() { _ = client.Close() }()

	ctx := context.Background()
	{
		err := client.Schema.Create(ctx)
		testutil.Require(t, !(err != nil), "ensure sekai schema: %v", err)
	}

	clientValue := reflect.ValueOf(client).Elem()
	clientType := clientValue.Type()
	queriedClients := make([]string, 0, len(migrate.Tables))

	for i := 0; i < clientValue.NumField(); i++ {
		fieldType := clientType.Field(i)
		if fieldType.PkgPath != "" || fieldType.Name == "Schema" {
			continue
		}

		fieldValue := clientValue.Field(i)
		if fieldValue.Kind() != reflect.Pointer || fieldValue.IsNil() {
			continue
		}

		queryMethod := fieldValue.MethodByName("Query")
		if !queryMethod.IsValid() {
			continue
		}

		queryResult := queryMethod.Call(nil)
		testutil.Require(t, !(len(queryResult) != 1), "%s.Query returned %d values, expect 1", fieldType.Name, len(queryResult))

		countMethod := queryResult[0].MethodByName("Count")
		testutil.Require(t, countMethod.IsValid(), "%s query has no Count method", fieldType.Name)

		countResult := countMethod.Call([]reflect.Value{reflect.ValueOf(ctx)})
		testutil.Require(t, !(len(countResult) != 2), "%s.Query().Count returned %d values, expect 2", fieldType.Name, len(countResult))
		testutil.Require(t, countResult[1].IsNil(), "%s.Query().Count failed: %v", fieldType.Name, countResult[1].Interface())

		queriedClients = append(queriedClients, fieldType.Name)
	}

	if len(queriedClients) != len(migrate.Tables) {
		sort.Strings(queriedClients)
		t.Fatalf("queried %d tables, expect %d; queried clients: %v", len(queriedClients), len(migrate.Tables), queriedClients)
	}

	t.Logf("queried all sekai tables successfully: %d", len(queriedClients))
}
