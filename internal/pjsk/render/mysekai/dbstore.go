package mysekai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"haruki-cloud/internal/observability/commandtrace"
)

var fileToTable = map[string]string{
	"mysekaiFixtures.json":                                    "mysekaifixtures",
	"customMusicScoreTags.json":                               "custommusicscoretags",
	"mysekaiFixtureMainGenres.json":                           "mysekaifixturemaingenres",
	"mysekaiFixtureSubGenres.json":                            "mysekaifixturesubgenres",
	"mysekaiBlueprints.json":                                  "mysekaiblueprints",
	"mysekaiBlueprintMysekaiMaterialCosts.json":               "mysekaiblueprintmysekaimaterialcosts",
	"mysekaiFixtureOnlyDisassembleMaterials.json":             "mysekaifixtureonlydisassemblematerials",
	"mysekaiFixtureTags.json":                                 "mysekaifixturetags",
	"mysekaiGateMaterialGroups.json":                          "mysekaigatematerialgroups",
	"mysekaiGateCharacterLotteries.json":                      "mysekaigatecharacterlotteries",
	"mysekaiGateCommonSkins.json":                             "mysekaigatecommonskins",
	"mysekaiGates.json":                                       "mysekaigates",
	"mysekaiGateSkins.json":                                   "mysekaigateskins",
	"mysekaiGateUnitSkins.json":                               "mysekaigateunitskins",
	"mysekaiHousingCompetitions.json":                         "mysekaihousingcompetitions",
	"mysekaiPhenomenas.json":                                  "mysekaiphenomenas",
	"mysekaiPhenomenaBackgroundColors.json":                   "mysekaiphenomenabackgroundcolors",
	"mysekaiMusicRecords.json":                                "mysekaimusicrecords",
	"mysekaiCharacterTalkConditions.json":                     "mysekaicharactertalkconditions",
	"mysekaiCharacterTalkConditionGroups.json":                "mysekaicharactertalkconditiongroups",
	"mysekaiCharacterTalks.json":                              "mysekaicharactertalks",
	"mysekaiGameCharacterUnitGroups.json":                     "mysekaigamecharacterunitgroups",
	"characterArchiveMysekaiCharacterTalkGroups.json":         "characterarchivemysekaicharactertalkgroups",
	"mysekaiMaterials.json":                                   "mysekaimaterials",
	"mysekaiItems.json":                                       "mysekaiitems",
	"mysekaiSiteHarvestFixtures.json":                         "mysekaisiteharvestfixtures",
	"mysekaiCustomFixtures.json":                              "mysekaicustomfixtures",
	"mysekaiRankReleases.json":                                "mysekairankreleases",
	"mysekaiSiteLevels.json":                                  "mysekaisitelevels",
	"mysekaiSiteLayouts.json":                                 "mysekaisitelayouts",
	"gameCharacters.json":                                     "gamecharacters",
	"gameCharacterUnits.json":                                 "gamecharacterunits",
	"cards.json":                                              "cards",
	"musics.json":                                             "musics",
	"musicTags.json":                                          "musictags",
	"limitedTimeMusics.json":                                  "limitedtimemusics",
	"mysekaiFixtureGameCharacterGroups.json":                  "mysekaifixturegamecharactergroups",
	"mysekaiFixtureGameCharacterGroupPerformanceBonuses.json": "mysekaifixturegamecharactergroupperformancebonuses",
	"mysekaiMaterialGameCharacterRelations.json":              "mysekaimaterialgamecharacterrelations",
}

// dbMasterdataStore queries the sekai PostgreSQL database instead of reading
// local JSON files.  It presents the same map-based interface that the
// controller expects.
type dbMasterdataStore struct {
	db     *sql.DB
	region string
	ctx    context.Context
	cache  *dbMasterdataCache
}

type dbMasterdataCache struct {
	mu         sync.Mutex
	lists      map[string][]map[string]any
	mapsByID   map[string]map[int]map[string]any
	generation uint64
}

// newDBMasterdataStore opens a read-only connection to the sekai database
// and returns a store scoped to the given server region.
// Returns nil if the DSN is empty or the connection fails.
func newDBMasterdataStore(ctx context.Context, dsn, region string) *dbMasterdataStore {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil
	}
	finishPing := commandtrace.MeasureOperation(ctx, "mysekai.masterdata_ping")
	err = db.PingContext(ctx)
	finishPing()
	if err != nil {
		db.Close()
		return nil
	}
	db.SetMaxOpenConns(3)
	return &dbMasterdataStore{
		db:     db,
		region: region,
		ctx:    ctx,
		cache: &dbMasterdataCache{
			lists:    make(map[string][]map[string]any),
			mapsByID: make(map[string]map[int]map[string]any),
		},
	}
}

func (s *dbMasterdataStore) Configured() bool {
	return s != nil && s.db != nil
}

func (s *dbMasterdataStore) resetCache() {
	if s == nil || s.cache == nil {
		return
	}
	s.cache.mu.Lock()
	s.cache.lists = make(map[string][]map[string]any)
	s.cache.mapsByID = make(map[string]map[int]map[string]any)
	s.cache.generation++
	s.cache.mu.Unlock()
}

func (s *dbMasterdataStore) Close() {
	if s != nil && s.db != nil {
		s.db.Close()
	}
}

func (s *dbMasterdataStore) WithContext(ctx context.Context) masterdataSource {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *dbMasterdataStore) contextOrBackground() context.Context {
	if s != nil && s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *dbMasterdataStore) loadList(filename string) []map[string]any {
	if s == nil || s.db == nil {
		return nil
	}

	if s.cache == nil {
		return nil
	}
	s.cache.mu.Lock()
	if cached, ok := s.cache.lists[filename]; ok {
		s.cache.mu.Unlock()
		return cached
	}
	generation := s.cache.generation
	s.cache.mu.Unlock()

	tableName, ok := fileToTable[filename]
	if !ok {
		return nil
	}

	items, err := s.queryTable(s.contextOrBackground(), tableName)
	if err != nil {
		return nil
	}

	s.cache.mu.Lock()
	if generation != s.cache.generation {
		s.cache.mu.Unlock()
		return s.loadList(filename)
	}
	if cached, ok := s.cache.lists[filename]; ok {
		s.cache.mu.Unlock()
		return cached
	}
	s.cache.lists[filename] = items
	s.cache.mu.Unlock()
	return items
}

func (s *dbMasterdataStore) loadMapByID(filename string) map[int]map[string]any {
	if s == nil || s.db == nil {
		return map[int]map[string]any{}
	}

	if s.cache == nil {
		return map[int]map[string]any{}
	}
	s.cache.mu.Lock()
	if cached, ok := s.cache.mapsByID[filename]; ok {
		s.cache.mu.Unlock()
		return cached
	}
	generation := s.cache.generation
	s.cache.mu.Unlock()

	items := s.loadList(filename)
	result := make(map[int]map[string]any, len(items))
	for _, item := range items {
		id := intNumber(item["id"], 0)
		if id == 0 {
			continue
		}
		result[id] = item
	}

	s.cache.mu.Lock()
	if generation != s.cache.generation {
		s.cache.mu.Unlock()
		return s.loadMapByID(filename)
	}
	if cached, ok := s.cache.mapsByID[filename]; ok {
		s.cache.mu.Unlock()
		return cached
	}
	s.cache.mapsByID[filename] = result
	s.cache.mu.Unlock()
	return result
}

// loadObject is not supported by the DB store; the only caller
// (fixture_reaction_data.json) has no corresponding table.
func (s *dbMasterdataStore) loadObject(_ string, _ any) bool {
	return false
}

// queryTable runs SELECT * on the given table filtered by server_region and
// converts each row into a map with camelCase keys matching the original
// game masterdata JSON format.
func (s *dbMasterdataStore) queryTable(ctx context.Context, table string) ([]map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	finishQuery := commandtrace.MeasureOperation(ctx, "mysekai.masterdata_query")
	defer finishQuery()

	// Use double-quoting for safety; table names are from our own constant map.
	query := fmt.Sprintf(`SELECT * FROM "%s" WHERE server_region = $1`, table)
	rows, err := s.db.QueryContext(ctx, query, s.region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, _ := rows.ColumnTypes()

	var results []map[string]any
	var decodeElapsed time.Duration
	defer func() {
		commandtrace.RecordOperation(ctx, "mysekai.masterdata_decode", decodeElapsed)
	}()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}

		decodeStarted := time.Now()
		m := make(map[string]any, len(cols))
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
		decodeElapsed += time.Since(decodeStarted)
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
func normalizeValue(val any, colTypes []*sql.ColumnType, idx int) any {
	switch v := val.(type) {
	case []byte:
		// Could be JSONB or TEXT; try JSON decode first.
		var parsed any
		if err := decodeJSONUseNumber(v, &parsed); err == nil {
			return parsed
		}
		return string(v)
	case int64:
		// Database numeric columns already arrive with integer precision.
		return v
	default:
		return v
	}
}
