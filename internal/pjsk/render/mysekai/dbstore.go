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
	"haruki-cloud/internal/pjsk/render/cachefill"
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
	fill       cachefill.Group
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
	s.cache.fill.Reset()
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
	items, _ := s.loadListChecked(filename)
	return items
}

// loadListChecked serves one table, caching it until the next reset. ok is
// false when the table is unmapped or the query failed. Concurrent callers
// share one fill, which runs detached from the request so a client that
// disconnects mid-query cannot leave a partial table behind (the trace
// values on the context are kept). A failed fill is not cached and is not
// retried until the fill backoff elapses.
func (s *dbMasterdataStore) loadListChecked(filename string) ([]map[string]any, bool) {
	if s == nil || s.db == nil {
		return nil, false
	}

	if s.cache == nil {
		return nil, false
	}
	s.cache.mu.Lock()
	cached, ok := s.cache.lists[filename]
	s.cache.mu.Unlock()
	if ok {
		return cached, true
	}

	tableName, ok := fileToTable[filename]
	if !ok {
		return nil, false
	}

	err := s.cache.fill.Do(s.contextOrBackground(), filename, func(fillCtx context.Context) error {
		s.cache.mu.Lock()
		_, cached := s.cache.lists[filename]
		generation := s.cache.generation
		s.cache.mu.Unlock()
		if cached {
			return nil
		}

		items, err := s.queryTable(fillCtx, tableName)
		if err != nil {
			return err
		}

		s.cache.mu.Lock()
		defer s.cache.mu.Unlock()
		if generation != s.cache.generation {
			// Reset while the query ran: the rows may predate the ingest.
			return nil
		}
		if _, cached := s.cache.lists[filename]; !cached {
			s.cache.lists[filename] = items
		}
		return nil
	})
	if err != nil {
		return nil, false
	}

	s.cache.mu.Lock()
	cached, ok = s.cache.lists[filename]
	s.cache.mu.Unlock()
	if !ok {
		return s.loadListChecked(filename)
	}
	return cached, true
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

	items, ok := s.loadListChecked(filename)
	if !ok {
		// Nothing was served: do not cache an empty index for a failed fill.
		return map[int]map[string]any{}
	}
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

	rows, err := queryMasterdataTable(ctx, s.db, table, s.region)
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

// Keep complete statements in this whitelist because SQL identifiers cannot be
// passed as bind parameters.
var masterdataTableQueries = map[string]string{
	"cards": `SELECT * FROM "cards" WHERE server_region = $1`,
	"characterarchivemysekaicharactertalkgroups":         `SELECT * FROM "characterarchivemysekaicharactertalkgroups" WHERE server_region = $1`,
	"custommusicscoretags":                               `SELECT * FROM "custommusicscoretags" WHERE server_region = $1`,
	"gamecharacters":                                     `SELECT * FROM "gamecharacters" WHERE server_region = $1`,
	"gamecharacterunits":                                 `SELECT * FROM "gamecharacterunits" WHERE server_region = $1`,
	"limitedtimemusics":                                  `SELECT * FROM "limitedtimemusics" WHERE server_region = $1`,
	"musics":                                             `SELECT * FROM "musics" WHERE server_region = $1`,
	"musictags":                                          `SELECT * FROM "musictags" WHERE server_region = $1`,
	"mysekaiblueprintmysekaimaterialcosts":               `SELECT * FROM "mysekaiblueprintmysekaimaterialcosts" WHERE server_region = $1`,
	"mysekaiblueprints":                                  `SELECT * FROM "mysekaiblueprints" WHERE server_region = $1`,
	"mysekaicharactertalkconditiongroups":                `SELECT * FROM "mysekaicharactertalkconditiongroups" WHERE server_region = $1`,
	"mysekaicharactertalkconditions":                     `SELECT * FROM "mysekaicharactertalkconditions" WHERE server_region = $1`,
	"mysekaicharactertalks":                              `SELECT * FROM "mysekaicharactertalks" WHERE server_region = $1`,
	"mysekaicustomfixtures":                              `SELECT * FROM "mysekaicustomfixtures" WHERE server_region = $1`,
	"mysekaifixturegamecharactergroupperformancebonuses": `SELECT * FROM "mysekaifixturegamecharactergroupperformancebonuses" WHERE server_region = $1`,
	"mysekaifixturegamecharactergroups":                  `SELECT * FROM "mysekaifixturegamecharactergroups" WHERE server_region = $1`,
	"mysekaifixturemaingenres":                           `SELECT * FROM "mysekaifixturemaingenres" WHERE server_region = $1`,
	"mysekaifixtureonlydisassemblematerials":             `SELECT * FROM "mysekaifixtureonlydisassemblematerials" WHERE server_region = $1`,
	"mysekaifixtures":                                    `SELECT * FROM "mysekaifixtures" WHERE server_region = $1`,
	"mysekaifixturesubgenres":                            `SELECT * FROM "mysekaifixturesubgenres" WHERE server_region = $1`,
	"mysekaifixturetags":                                 `SELECT * FROM "mysekaifixturetags" WHERE server_region = $1`,
	"mysekaigamecharacterunitgroups":                     `SELECT * FROM "mysekaigamecharacterunitgroups" WHERE server_region = $1`,
	"mysekaigatecharacterlotteries":                      `SELECT * FROM "mysekaigatecharacterlotteries" WHERE server_region = $1`,
	"mysekaigatecommonskins":                             `SELECT * FROM "mysekaigatecommonskins" WHERE server_region = $1`,
	"mysekaigatematerialgroups":                          `SELECT * FROM "mysekaigatematerialgroups" WHERE server_region = $1`,
	"mysekaigates":                                       `SELECT * FROM "mysekaigates" WHERE server_region = $1`,
	"mysekaigateskins":                                   `SELECT * FROM "mysekaigateskins" WHERE server_region = $1`,
	"mysekaigateunitskins":                               `SELECT * FROM "mysekaigateunitskins" WHERE server_region = $1`,
	"mysekaihousingcompetitions":                         `SELECT * FROM "mysekaihousingcompetitions" WHERE server_region = $1`,
	"mysekaiitems":                                       `SELECT * FROM "mysekaiitems" WHERE server_region = $1`,
	"mysekaimaterialgamecharacterrelations":              `SELECT * FROM "mysekaimaterialgamecharacterrelations" WHERE server_region = $1`,
	"mysekaimaterials":                                   `SELECT * FROM "mysekaimaterials" WHERE server_region = $1`,
	"mysekaimusicrecords":                                `SELECT * FROM "mysekaimusicrecords" WHERE server_region = $1`,
	"mysekaiphenomenabackgroundcolors":                   `SELECT * FROM "mysekaiphenomenabackgroundcolors" WHERE server_region = $1`,
	"mysekaiphenomenas":                                  `SELECT * FROM "mysekaiphenomenas" WHERE server_region = $1`,
	"mysekairankreleases":                                `SELECT * FROM "mysekairankreleases" WHERE server_region = $1`,
	"mysekaisiteharvestfixtures":                         `SELECT * FROM "mysekaisiteharvestfixtures" WHERE server_region = $1`,
	"mysekaisitelayouts":                                 `SELECT * FROM "mysekaisitelayouts" WHERE server_region = $1`,
	"mysekaisitelevels":                                  `SELECT * FROM "mysekaisitelevels" WHERE server_region = $1`,
}

func queryMasterdataTable(ctx context.Context, db *sql.DB, table, region string) (*sql.Rows, error) {
	query, ok := masterdataTableQueries[table]
	if !ok {
		return nil, fmt.Errorf("unsupported MySekai masterdata table %q", table)
	}
	return db.QueryContext(ctx, query, region)
}

// mapColumnName converts a DB column name to the camelCase key the controller
// expects. Returns "" for columns that should be excluded from the map.
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
