package sekai_test

import (
	"context"
	"os"
	"reflect"
	"sort"
	"testing"

	_ "github.com/lib/pq"
	entsekai "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/migrate"
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
	if err != nil {
		t.Fatalf("open sekai client: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("ensure sekai schema: %v", err)
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
		if len(queryResult) != 1 {
			t.Fatalf("%s.Query returned %d values, expect 1", fieldType.Name, len(queryResult))
		}

		countMethod := queryResult[0].MethodByName("Count")
		if !countMethod.IsValid() {
			t.Fatalf("%s query has no Count method", fieldType.Name)
		}

		countResult := countMethod.Call([]reflect.Value{reflect.ValueOf(ctx)})
		if len(countResult) != 2 {
			t.Fatalf("%s.Query().Count returned %d values, expect 2", fieldType.Name, len(countResult))
		}
		if !countResult[1].IsNil() {
			t.Fatalf("%s.Query().Count failed: %v", fieldType.Name, countResult[1].Interface())
		}

		queriedClients = append(queriedClients, fieldType.Name)
	}

	if len(queriedClients) != len(migrate.Tables) {
		sort.Strings(queriedClients)
		t.Fatalf("queried %d tables, expect %d; queried clients: %v", len(queriedClients), len(migrate.Tables), queriedClients)
	}

	t.Logf("queried all sekai tables successfully: %d", len(queriedClients))
}
