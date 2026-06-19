package provider

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"unicode"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
)

var mysekaiFileToTable = map[string]string{
	"cards.json": "cards",
	"characterArchiveMysekaiCharacterTalkGroups.json":         "characterarchivemysekaicharactertalkgroups",
	"customMusicScoreTags.json":                               "custommusicscoretags",
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
}

type dbMySekaiProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	local  *localMySekaiProvider

	db          *sql.DB
	dbType      string
	mu          sync.Mutex
	lists       map[string][]map[string]any
	mapsByID    map[string]map[int]map[string]any
	unavailable map[string]struct{}
}

func newDBMySekaiProvider(client *sekaiDB.Client, region renderregion.Value, cfg databaseProviderConfig) *dbMySekaiProvider {
	p := &dbMySekaiProvider{
		client:      client,
		region:      region,
		dbType:      strings.TrimSpace(cfg.sekaiDBType),
		lists:       make(map[string][]map[string]any),
		mapsByID:    make(map[string]map[int]map[string]any),
		unavailable: make(map[string]struct{}),
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
	if p == nil {
		return nil
	}
	if items, ok := p.loadDBList(filename); ok {
		return items
	}
	if p.local == nil {
		return nil
	}
	return p.local.LoadList(filename)
}

func (p *dbMySekaiProvider) LoadMapByID(filename string) map[int]map[string]any {
	if p == nil {
		return nil
	}
	if items, ok := p.loadDBMapByID(filename); ok {
		return items
	}
	if p.local == nil {
		return nil
	}
	return p.local.LoadMapByID(filename)
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

func (p *dbMySekaiProvider) loadDBList(filename string) ([]map[string]any, bool) {
	if p == nil || p.db == nil {
		return nil, false
	}
	table, ok := mysekaiFileToTable[filename]
	if !ok {
		return nil, false
	}

	p.mu.Lock()
	if items, ok := p.lists[filename]; ok {
		p.mu.Unlock()
		return items, true
	}
	if _, ok := p.unavailable[filename]; ok {
		p.mu.Unlock()
		return nil, false
	}
	p.mu.Unlock()

	items, err := p.queryTable(table)
	if err != nil {
		p.mu.Lock()
		p.unavailable[filename] = struct{}{}
		p.mu.Unlock()
		return nil, false
	}

	p.mu.Lock()
	p.lists[filename] = items
	p.mu.Unlock()
	return items, true
}

func (p *dbMySekaiProvider) loadDBMapByID(filename string) (map[int]map[string]any, bool) {
	if p == nil || p.db == nil {
		return nil, false
	}

	p.mu.Lock()
	if cached, ok := p.mapsByID[filename]; ok {
		p.mu.Unlock()
		return cached, true
	}
	p.mu.Unlock()

	items, ok := p.loadDBList(filename)
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
	p.mapsByID[filename] = result
	p.mu.Unlock()
	return result, true
}

func (p *dbMySekaiProvider) queryTable(table string) ([]map[string]any, error) {
	placeholder := "?"
	if strings.EqualFold(p.dbType, "postgres") {
		placeholder = "$1"
	}
	query := fmt.Sprintf(`SELECT * FROM "%s" WHERE server_region = %s`, table, placeholder)
	rows, err := p.db.Query(query, renderregion.WithDefault(p.region).String())
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
