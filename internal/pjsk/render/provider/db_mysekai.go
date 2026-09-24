package provider

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"unicode"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/cachefill"
)

// mysekaiFileToTable maps a master JSON file to the table that holds its
// rows. Besides the MySekai tables it lists the tables Cloud forwards as
// whole rows (custom profile resources, omikujis, unit story episode groups,
// event stories), which need the camelCase map shape rather than a typed
// model.
var mysekaiFileToTable = map[string]string{
	"cards.json": "cards",
	"characterArchiveMysekaiCharacterTalkGroups.json":         "characterarchivemysekaicharactertalkgroups",
	"customMusicScoreTags.json":                               "custommusicscoretags",
	"customProfileCharacterIconResources.json":                "customprofilecharactericonresources",
	"customProfileCollectionResources.json":                   "customprofilecollectionresources",
	"customProfileEtcResources.json":                          "customprofileetcresources",
	"customProfileGeneralBackgroundResources.json":            "customprofilegeneralbackgroundresources",
	"customProfileMaterialResources.json":                     "customprofilematerialresources",
	"customProfileMemberStandingPictureResources.json":        "customprofilememberstandingpictureresources",
	"customProfilePlayerInfoResources.json":                   "customprofileplayerinforesources",
	"customProfileShapeResources.json":                        "customprofileshaperesources",
	"customProfileStoryBackgroundResources.json":              "customprofilestorybackgroundresources",
	"customProfileTextColors.json":                            "customprofiletextcolors",
	"customProfileTextFonts.json":                             "customprofiletextfonts",
	"customProfileUserInterfaceIconResources.json":            "customprofileuserinterfaceiconresources",
	"eventStories.json":                                       "eventstories",
	"gameCharacters.json":                                     "gamecharacters",
	"gameCharacterUnits.json":                                 "gamecharacterunits",
	"limitedTimeMusics.json":                                  "limitedtimemusics",
	"musicTags.json":                                          "musictags",
	"musics.json":                                             "musics",
	"mysekaiBlueprintMysekaiMaterialCosts.json":               "mysekaiblueprintmysekaimaterialcosts",
	"mysekaiBlueprints.json":                                  "mysekaiblueprints",
	"mysekaiCharacterTalkConditionGroups.json":                "mysekaicharactertalkconditiongroups",
	"mysekaiCharacterTalkConditions.json":                     "mysekaicharactertalkconditions",
	"mysekaiCharacterTalks.json":                              "mysekaicharactertalks",
	"mysekaiCustomFixtures.json":                              "mysekaicustomfixtures",
	"mysekaiFixtureGameCharacterGroupPerformanceBonuses.json": "mysekaifixturegamecharactergroupperformancebonuses",
	"mysekaiFixtureGameCharacterGroups.json":                  "mysekaifixturegamecharactergroups",
	"mysekaiFixtureMainGenres.json":                           "mysekaifixturemaingenres",
	"mysekaiFixtureOnlyDisassembleMaterials.json":             "mysekaifixtureonlydisassemblematerials",
	"mysekaiFixtureSubGenres.json":                            "mysekaifixturesubgenres",
	"mysekaiFixtureTags.json":                                 "mysekaifixturetags",
	"mysekaiFixtures.json":                                    "mysekaifixtures",
	"mysekaiGameCharacterUnitGroups.json":                     "mysekaigamecharacterunitgroups",
	"mysekaiGateCharacterLotteries.json":                      "mysekaigatecharacterlotteries",
	"mysekaiGateCommonSkins.json":                             "mysekaigatecommonskins",
	"mysekaiGateMaterialGroups.json":                          "mysekaigatematerialgroups",
	"mysekaiGateSkins.json":                                   "mysekaigateskins",
	"mysekaiGateUnitSkins.json":                               "mysekaigateunitskins",
	"mysekaiGates.json":                                       "mysekaigates",
	"mysekaiHousingCompetitions.json":                         "mysekaihousingcompetitions",
	"mysekaiItems.json":                                       "mysekaiitems",
	"mysekaiMaterialGameCharacterRelations.json":              "mysekaimaterialgamecharacterrelations",
	"mysekaiMaterials.json":                                   "mysekaimaterials",
	"mysekaiMusicRecords.json":                                "mysekaimusicrecords",
	"mysekaiPhenomenaBackgroundColors.json":                   "mysekaiphenomenabackgroundcolors",
	"mysekaiPhenomenas.json":                                  "mysekaiphenomenas",
	"mysekaiRankReleases.json":                                "mysekairankreleases",
	"mysekaiSiteHarvestFixtures.json":                         "mysekaisiteharvestfixtures",
	"mysekaiSiteLayouts.json":                                 "mysekaisitelayouts",
	"mysekaiSiteLevels.json":                                  "mysekaisitelevels",
	"omikujis.json":                                           "omikujis",
	"unitStoryEpisodeGroups.json":                             "unitstoryepisodegroups",
}

// MasterRowSource is implemented by row stores that can say whether a master
// table was served at all. ok is false only when neither the database nor a
// local store answered (no database, query error, or missing file); a served
// but empty table reports ok with no rows.
type MasterRowSource interface {
	LoadMasterRows(ctx context.Context, filename string) (rows map[int]map[string]any, ok bool)
}

type dbMySekaiProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	local  *localMySekaiProvider

	db       *sql.DB
	dbType   string
	mu       sync.Mutex
	lists    map[string][]map[string]any
	mapsByID map[string]map[int]map[string]any
	fill     cachefill.Group
}

func newDBMySekaiProvider(client *sekaiDB.Client, region renderregion.Value, cfg databaseProviderConfig) *dbMySekaiProvider {
	p := &dbMySekaiProvider{
		client:   client,
		region:   region,
		dbType:   strings.TrimSpace(cfg.sekaiDBType),
		lists:    make(map[string][]map[string]any),
		mapsByID: make(map[string]map[int]map[string]any),
	}
	if p.dbType == "" {
		p.dbType = "postgres"
	}
	dsn := strings.TrimSpace(cfg.sekaiDSN)
	if dsn == "" {
		return p
	}
	db, err := sql.Open(p.dbType, dsn)
	if err != nil {
		return p
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return p
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	p.db = db
	return p
}

func (p *dbMySekaiProvider) Configured() bool {
	return p != nil && (p.db != nil || p.local != nil && p.local.Configured())
}

func (p *dbMySekaiProvider) LoadList(filename string) []map[string]any {
	return p.LoadListContext(context.Background(), filename)
}

// LoadListContext serves a table from the database first. A served but
// empty table falls back to the local store when one is configured (the
// table exists before its first ingest), and a database that cannot answer
// falls back the same way.
func (p *dbMySekaiProvider) LoadListContext(ctx context.Context, filename string) []map[string]any {
	if p == nil {
		return nil
	}
	items, ok := p.loadDBList(ctx, filename)
	if ok && len(items) > 0 {
		return items
	}
	if p.local != nil {
		if fallback := p.local.LoadList(filename); fallback != nil {
			return fallback
		}
	}
	if ok {
		return items
	}
	return nil
}

func (p *dbMySekaiProvider) LoadMapByID(filename string) map[int]map[string]any {
	return p.LoadMapByIDContext(context.Background(), filename)
}

func (p *dbMySekaiProvider) LoadMapByIDContext(ctx context.Context, filename string) map[int]map[string]any {
	rows, _ := p.LoadMasterRows(ctx, filename)
	return rows
}

// LoadMasterRows implements MasterRowSource with the LoadListContext
// precedence: database rows, else the local store, else the served empty
// table.
func (p *dbMySekaiProvider) LoadMasterRows(ctx context.Context, filename string) (map[int]map[string]any, bool) {
	if p == nil {
		return nil, false
	}
	items, ok := p.loadDBMapByID(ctx, filename)
	if ok && len(items) > 0 {
		return items, true
	}
	if p.local != nil {
		if fallback := p.local.LoadMapByID(filename); fallback != nil {
			return fallback, true
		}
	}
	if ok {
		return items, true
	}
	return nil, false
}

func (p *dbMySekaiProvider) LoadObject(filename string, target any) bool {
	if p == nil || p.local == nil {
		return false
	}
	return p.local.LoadObject(filename, target)
}

func (p *dbMySekaiProvider) Close() error {
	if p == nil || p.db == nil {
		return nil
	}
	return p.db.Close()
}

// loadDBList serves one table from the database. The rows are cached until
// the masterdata cache is reset (the registry poll resets it after an
// ingest). Concurrent callers share one query, and a query error is not
// cached: the table is not served again until the fill backoff elapses,
// after which the next call retries.
func (p *dbMySekaiProvider) loadDBList(ctx context.Context, filename string) ([]map[string]any, bool) {
	if p == nil || p.db == nil {
		return nil, false
	}
	table, ok := mysekaiFileToTable[filename]
	if !ok {
		return nil, false
	}

	p.mu.Lock()
	items, ok := p.lists[filename]
	p.mu.Unlock()
	if ok {
		return items, true
	}

	err := p.fill.Do(ctx, filename, func(fillCtx context.Context) error {
		p.mu.Lock()
		_, cached := p.lists[filename]
		p.mu.Unlock()
		if cached {
			return nil
		}
		items, err := p.queryTable(fillCtx, table)
		if err != nil {
			return err
		}
		p.mu.Lock()
		if _, cached := p.lists[filename]; !cached {
			p.lists[filename] = items
		}
		p.mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, false
	}

	p.mu.Lock()
	items, ok = p.lists[filename]
	p.mu.Unlock()
	if !ok {
		// The cache was reset while the fill ran; fill again.
		return p.loadDBList(ctx, filename)
	}
	return items, true
}

func (p *dbMySekaiProvider) loadDBMapByID(ctx context.Context, filename string) (map[int]map[string]any, bool) {
	if p == nil || p.db == nil {
		return nil, false
	}

	p.mu.Lock()
	if cached, ok := p.mapsByID[filename]; ok {
		p.mu.Unlock()
		return cached, true
	}
	p.mu.Unlock()

	items, ok := p.loadDBList(ctx, filename)
	if !ok {
		return nil, false
	}
	result := make(map[int]map[string]any, len(items))
	for _, item := range items {
		if id, ok := interfaceToInt(item["id"]); ok && id != 0 {
			result[id] = item
		}
	}

	p.mu.Lock()
	if cached, ok := p.mapsByID[filename]; ok {
		p.mu.Unlock()
		return cached, true
	}
	p.mapsByID[filename] = result
	p.mu.Unlock()
	return result, true
}

func (p *dbMySekaiProvider) queryTable(ctx context.Context, table string) ([]map[string]any, error) {
	rows, err := queryMySekaiTable(ctx, p.db, p.dbType, table, renderregion.WithDefault(p.region).String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, _ := rows.ColumnTypes()

	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		item := make(map[string]any, len(cols))
		for i, col := range cols {
			key := mysekaiColumnKey(col)
			if key == "" || values[i] == nil {
				continue
			}
			item[key] = normalizeDBMasterdataValue(values[i], colTypes, i)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Keep complete statements in these whitelists because SQL identifiers cannot
// be passed as bind parameters.
var mysekaiPostgresTableQueries = map[string]string{
	"cards": `SELECT * FROM "cards" WHERE server_region = $1`,
	"characterarchivemysekaicharactertalkgroups":         `SELECT * FROM "characterarchivemysekaicharactertalkgroups" WHERE server_region = $1`,
	"custommusicscoretags":                               `SELECT * FROM "custommusicscoretags" WHERE server_region = $1`,
	"customprofilecharactericonresources":                `SELECT * FROM "customprofilecharactericonresources" WHERE server_region = $1`,
	"customprofilecollectionresources":                   `SELECT * FROM "customprofilecollectionresources" WHERE server_region = $1`,
	"customprofileetcresources":                          `SELECT * FROM "customprofileetcresources" WHERE server_region = $1`,
	"customprofilegeneralbackgroundresources":            `SELECT * FROM "customprofilegeneralbackgroundresources" WHERE server_region = $1`,
	"customprofilematerialresources":                     `SELECT * FROM "customprofilematerialresources" WHERE server_region = $1`,
	"customprofilememberstandingpictureresources":        `SELECT * FROM "customprofilememberstandingpictureresources" WHERE server_region = $1`,
	"customprofileplayerinforesources":                   `SELECT * FROM "customprofileplayerinforesources" WHERE server_region = $1`,
	"customprofileshaperesources":                        `SELECT * FROM "customprofileshaperesources" WHERE server_region = $1`,
	"customprofilestorybackgroundresources":              `SELECT * FROM "customprofilestorybackgroundresources" WHERE server_region = $1`,
	"customprofiletextcolors":                            `SELECT * FROM "customprofiletextcolors" WHERE server_region = $1`,
	"customprofiletextfonts":                             `SELECT * FROM "customprofiletextfonts" WHERE server_region = $1`,
	"customprofileuserinterfaceiconresources":            `SELECT * FROM "customprofileuserinterfaceiconresources" WHERE server_region = $1`,
	"eventstories":                                       `SELECT * FROM "eventstories" WHERE server_region = $1`,
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
	"omikujis":                                           `SELECT * FROM "omikujis" WHERE server_region = $1`,
	"unitstoryepisodegroups":                             `SELECT * FROM "unitstoryepisodegroups" WHERE server_region = $1`,
}

var mysekaiQuestionMarkTableQueries = map[string]string{
	"cards": `SELECT * FROM cards WHERE server_region = ?`,
	"characterarchivemysekaicharactertalkgroups":         `SELECT * FROM characterarchivemysekaicharactertalkgroups WHERE server_region = ?`,
	"custommusicscoretags":                               `SELECT * FROM custommusicscoretags WHERE server_region = ?`,
	"customprofilecharactericonresources":                `SELECT * FROM customprofilecharactericonresources WHERE server_region = ?`,
	"customprofilecollectionresources":                   `SELECT * FROM customprofilecollectionresources WHERE server_region = ?`,
	"customprofileetcresources":                          `SELECT * FROM customprofileetcresources WHERE server_region = ?`,
	"customprofilegeneralbackgroundresources":            `SELECT * FROM customprofilegeneralbackgroundresources WHERE server_region = ?`,
	"customprofilematerialresources":                     `SELECT * FROM customprofilematerialresources WHERE server_region = ?`,
	"customprofilememberstandingpictureresources":        `SELECT * FROM customprofilememberstandingpictureresources WHERE server_region = ?`,
	"customprofileplayerinforesources":                   `SELECT * FROM customprofileplayerinforesources WHERE server_region = ?`,
	"customprofileshaperesources":                        `SELECT * FROM customprofileshaperesources WHERE server_region = ?`,
	"customprofilestorybackgroundresources":              `SELECT * FROM customprofilestorybackgroundresources WHERE server_region = ?`,
	"customprofiletextcolors":                            `SELECT * FROM customprofiletextcolors WHERE server_region = ?`,
	"customprofiletextfonts":                             `SELECT * FROM customprofiletextfonts WHERE server_region = ?`,
	"customprofileuserinterfaceiconresources":            `SELECT * FROM customprofileuserinterfaceiconresources WHERE server_region = ?`,
	"eventstories":                                       `SELECT * FROM eventstories WHERE server_region = ?`,
	"gamecharacters":                                     `SELECT * FROM gamecharacters WHERE server_region = ?`,
	"gamecharacterunits":                                 `SELECT * FROM gamecharacterunits WHERE server_region = ?`,
	"limitedtimemusics":                                  `SELECT * FROM limitedtimemusics WHERE server_region = ?`,
	"musics":                                             `SELECT * FROM musics WHERE server_region = ?`,
	"musictags":                                          `SELECT * FROM musictags WHERE server_region = ?`,
	"mysekaiblueprintmysekaimaterialcosts":               `SELECT * FROM mysekaiblueprintmysekaimaterialcosts WHERE server_region = ?`,
	"mysekaiblueprints":                                  `SELECT * FROM mysekaiblueprints WHERE server_region = ?`,
	"mysekaicharactertalkconditiongroups":                `SELECT * FROM mysekaicharactertalkconditiongroups WHERE server_region = ?`,
	"mysekaicharactertalkconditions":                     `SELECT * FROM mysekaicharactertalkconditions WHERE server_region = ?`,
	"mysekaicharactertalks":                              `SELECT * FROM mysekaicharactertalks WHERE server_region = ?`,
	"mysekaicustomfixtures":                              `SELECT * FROM mysekaicustomfixtures WHERE server_region = ?`,
	"mysekaifixturegamecharactergroupperformancebonuses": `SELECT * FROM mysekaifixturegamecharactergroupperformancebonuses WHERE server_region = ?`,
	"mysekaifixturegamecharactergroups":                  `SELECT * FROM mysekaifixturegamecharactergroups WHERE server_region = ?`,
	"mysekaifixturemaingenres":                           `SELECT * FROM mysekaifixturemaingenres WHERE server_region = ?`,
	"mysekaifixtureonlydisassemblematerials":             `SELECT * FROM mysekaifixtureonlydisassemblematerials WHERE server_region = ?`,
	"mysekaifixtures":                                    `SELECT * FROM mysekaifixtures WHERE server_region = ?`,
	"mysekaifixturesubgenres":                            `SELECT * FROM mysekaifixturesubgenres WHERE server_region = ?`,
	"mysekaifixturetags":                                 `SELECT * FROM mysekaifixturetags WHERE server_region = ?`,
	"mysekaigamecharacterunitgroups":                     `SELECT * FROM mysekaigamecharacterunitgroups WHERE server_region = ?`,
	"mysekaigatecharacterlotteries":                      `SELECT * FROM mysekaigatecharacterlotteries WHERE server_region = ?`,
	"mysekaigatecommonskins":                             `SELECT * FROM mysekaigatecommonskins WHERE server_region = ?`,
	"mysekaigatematerialgroups":                          `SELECT * FROM mysekaigatematerialgroups WHERE server_region = ?`,
	"mysekaigates":                                       `SELECT * FROM mysekaigates WHERE server_region = ?`,
	"mysekaigateskins":                                   `SELECT * FROM mysekaigateskins WHERE server_region = ?`,
	"mysekaigateunitskins":                               `SELECT * FROM mysekaigateunitskins WHERE server_region = ?`,
	"mysekaihousingcompetitions":                         `SELECT * FROM mysekaihousingcompetitions WHERE server_region = ?`,
	"mysekaiitems":                                       `SELECT * FROM mysekaiitems WHERE server_region = ?`,
	"mysekaimaterialgamecharacterrelations":              `SELECT * FROM mysekaimaterialgamecharacterrelations WHERE server_region = ?`,
	"mysekaimaterials":                                   `SELECT * FROM mysekaimaterials WHERE server_region = ?`,
	"mysekaimusicrecords":                                `SELECT * FROM mysekaimusicrecords WHERE server_region = ?`,
	"mysekaiphenomenabackgroundcolors":                   `SELECT * FROM mysekaiphenomenabackgroundcolors WHERE server_region = ?`,
	"mysekaiphenomenas":                                  `SELECT * FROM mysekaiphenomenas WHERE server_region = ?`,
	"mysekairankreleases":                                `SELECT * FROM mysekairankreleases WHERE server_region = ?`,
	"mysekaisiteharvestfixtures":                         `SELECT * FROM mysekaisiteharvestfixtures WHERE server_region = ?`,
	"mysekaisitelayouts":                                 `SELECT * FROM mysekaisitelayouts WHERE server_region = ?`,
	"mysekaisitelevels":                                  `SELECT * FROM mysekaisitelevels WHERE server_region = ?`,
	"omikujis":                                           `SELECT * FROM omikujis WHERE server_region = ?`,
	"unitstoryepisodegroups":                             `SELECT * FROM unitstoryepisodegroups WHERE server_region = ?`,
}

func queryMySekaiTable(ctx context.Context, db *sql.DB, dbType, table, region string) (*sql.Rows, error) {
	queries := mysekaiQuestionMarkTableQueries
	if strings.EqualFold(dbType, "postgres") {
		queries = mysekaiPostgresTableQueries
	}
	query, ok := queries[table]
	if !ok {
		return nil, fmt.Errorf("unsupported MySekai masterdata table %q", table)
	}
	return db.QueryContext(ctx, query, region)
}

func mysekaiColumnKey(col string) string {
	switch col {
	case "id":
		return ""
	case "game_id":
		return "id"
	case "server_region":
		return ""
	default:
		return snakeToCamel(col)
	}
}

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

func normalizeDBMasterdataValue(value any, _ []*sql.ColumnType, _ int) any {
	switch v := value.(type) {
	case []byte:
		var parsed any
		if err := decodeJSONUseNumber(v, &parsed); err == nil {
			return parsed
		}
		return string(v)
	default:
		return v
	}
}
