package mysekai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// masterdataSource abstracts access to game masterdata so that the mysekai
// controller can read from either local JSON files or a database.
type masterdataSource interface {
	Configured() bool
	loadList(filename string) []map[string]interface{}
	loadMapByID(filename string) map[int]map[string]interface{}
	loadObject(filename string, target interface{}) bool
}

// fileToTable maps the original game masterdata JSON filenames to the
// corresponding PostgreSQL table names used by the sekai database.
var fileToTable = map[string]string{
	"mysekaiFixtures.json":                                      "mysekaifixtures",
	"mysekaiFixtureMainGenres.json":                             "mysekaifixturemaingenres",
	"mysekaiFixtureSubGenres.json":                              "mysekaifixturesubgenres",
	"mysekaiBlueprints.json":                                    "mysekaiblueprints",
	"mysekaiBlueprintMysekaiMaterialCosts.json":                 "mysekaiblueprintmysekaimaterialcosts",
	"mysekaiFixtureOnlyDisassembleMaterials.json":               "mysekaifixtureonlydisassemblematerials",
	"mysekaiFixtureTags.json":                                   "mysekaifixturetags",
	"mysekaiGateMaterialGroups.json":                            "mysekaigatematerialgroups",
	"mysekaiMusicRecords.json":                                  "mysekaimusicrecords",
	"mysekaiCharacterTalkConditions.json":                       "mysekaicharactertalkconditions",
	"mysekaiCharacterTalkConditionGroups.json":                  "mysekaicharactertalkconditiongroups",
	"mysekaiCharacterTalks.json":                                "mysekaicharactertalks",
	"mysekaiGameCharacterUnitGroups.json":                       "mysekaigamecharacterunitgroups",
	"characterArchiveMysekaiCharacterTalkGroups.json":           "characterarchivemysekaicharactertalkgroups",
	"mysekaiMaterials.json":                                     "mysekaimaterials",
	"mysekaiItems.json":                                         "mysekaiitems",
	"gameCharacters.json":                                       "gamecharacters",
	"gameCharacterUnits.json":                                   "gamecharacterunits",
	"musics.json":                                               "musics",
	"musicTags.json":                                            "musictags",
	"limitedTimeMusics.json":                                    "limitedtimemusics",
	"mysekaiFixtureGameCharacterGroups.json":                    "mysekaifixturegamecharactergroups",
	"mysekaiMaterialGameCharacterRelations.json":                "mysekaimaterialgamecharacterrelations",
}

// dbMasterdataStore queries the sekai PostgreSQL database instead of reading
// local JSON files.  It presents the same map-based interface that the
// controller expects.
type dbMasterdataStore struct {
	db     *sql.DB
	region string

	mu       sync.Mutex
	lists    map[string][]map[string]interface{}
	mapsByID map[string]map[int]map[string]interface{}
}

// newDBMasterdataStore opens a read-only connection to the sekai database
// and returns a store scoped to the given server region.
// Returns nil if the DSN is empty or the connection fails.
func newDBMasterdataStore(dsn, region string) *dbMasterdataStore {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil
	}
	db.SetMaxOpenConns(3)
	return &dbMasterdataStore{
		db:       db,
		region:   region,
		lists:    make(map[string][]map[string]interface{}),
		mapsByID: make(map[string]map[int]map[string]interface{}),
	}
}

func (s *dbMasterdataStore) Configured() bool {
	return s != nil && s.db != nil
}

func (s *dbMasterdataStore) Close() {
	if s != nil && s.db != nil {
		s.db.Close()
	}
}

func (s *dbMasterdataStore) loadList(filename string) []map[string]interface{} {
	if s == nil || s.db == nil {
		return nil
	}

	s.mu.Lock()
	if cached, ok := s.lists[filename]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	tableName, ok := fileToTable[filename]
	if !ok {
		return nil
	}

	items, err := s.queryTable(tableName)
	if err != nil {
		return nil
	}

	s.mu.Lock()
	s.lists[filename] = items
	s.mu.Unlock()
	return items
}

func (s *dbMasterdataStore) loadMapByID(filename string) map[int]map[string]interface{} {
	if s == nil || s.db == nil {
		return map[int]map[string]interface{}{}
	}

	s.mu.Lock()
	if cached, ok := s.mapsByID[filename]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	items := s.loadList(filename)
	result := make(map[int]map[string]interface{}, len(items))
	for _, item := range items {
		id := intNumber(item["id"], 0)
		if id == 0 {
			continue
		}
		result[id] = item
	}

	s.mu.Lock()
	s.mapsByID[filename] = result
	s.mu.Unlock()
	return result
}

// loadObject is not supported by the DB store; the only caller
// (fixture_reaction_data.json) has no corresponding table.
func (s *dbMasterdataStore) loadObject(_ string, _ interface{}) bool {
	return false
}

// queryTable runs SELECT * on the given table filtered by server_region and
// converts each row into a map with camelCase keys matching the original
// game masterdata JSON format.
func (s *dbMasterdataStore) queryTable(table string) ([]map[string]interface{}, error) {
	// Use double-quoting for safety; table names are from our own constant map.
	query := fmt.Sprintf(`SELECT * FROM "%s" WHERE server_region = $1`, table)
	rows, err := s.db.Query(query, s.region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, _ := rows.ColumnTypes()

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}

		m := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			key := mapColumnName(col)
			if key == "" {
				continue // skip id (auto-increment) and server_region
			}
			val := values[i]
			if val == nil {
				continue // omit null values to match omitempty behaviour
			}
			m[key] = normalizeValue(val, colTypes, i)
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// mapColumnName converts a DB column name to the camelCase key the controller
// expects.  Returns "" for columns that should be excluded from the map.
func mapColumnName(col string) string {
	switch col {
	case "id":
		return "" // Ent auto-increment, not game data
	case "game_id":
		return "id" // the game's primary key
	case "server_region":
		return "" // filtering column, not game data
	default:
		return snakeToCamel(col)
	}
}

// snakeToCamel converts "my_field_name" to "myFieldName".
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return s
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// normalizeValue converts a raw sql.Scan result into the type the controller
// expects (matching JSON unmarshal behaviour).
func normalizeValue(val interface{}, colTypes []*sql.ColumnType, idx int) interface{} {
	switch v := val.(type) {
	case []byte:
		// Could be JSONB or TEXT; try JSON decode first.
		var parsed interface{}
		if err := json.Unmarshal(v, &parsed); err == nil {
			return parsed
		}
		return string(v)
	case int64:
		// JSON numbers for small ints come through as float64 when
		// unmarshalled from JSON.  The controller uses intNumber() which
		// handles both int and float64, so int64 is fine.
		return v
	default:
		return v
	}
}
