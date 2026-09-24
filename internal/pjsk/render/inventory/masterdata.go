package inventory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	json "haruki-cloud/internal/jsonutil"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/boostitem"
	"haruki-cloud/database/sekai/eventitem"
	"haruki-cloud/database/sekai/gachaceilitem"
	"haruki-cloud/database/sekai/gachaticket"
	"haruki-cloud/database/sekai/material"
	"haruki-cloud/database/sekai/mysekaimaterial"
	"haruki-cloud/database/sekai/practiceticket"
	"haruki-cloud/database/sekai/skillpracticeticket"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/cachefill"
)

var (
	// errMasterdataFillIncomplete reports a region fill in which at least
	// one database query failed.
	errMasterdataFillIncomplete = errors.New("inventory masterdata fill incomplete")
	// errMasterdataFillStale reports a region fill discarded because the
	// cache was reset while it ran; the request fills again.
	errMasterdataFillStale = errors.New("inventory masterdata fill predates a reset")
)

func newMasterdataStore(client *sekaiDB.Client, localDir string) *masterdataStore {
	return &masterdataStore{
		client:   client,
		localDir: strings.TrimSpace(localDir),
		cache:    make(map[string]*regionMasterdata),
		partial:  make(map[string]*regionMasterdata),
		// Keys are regions, so the failure log's key names the region.
		fill: cachefill.Group{Cache: "inventory"},
	}
}

// forRegion serves a region's inventory tables, filling them once from the
// database (and local files for the tables it serves empty). The fill runs
// detached from the request context and is shared by concurrent requests;
// a fill that straddles a cache reset is discarded and run again so rows
// read before an ingest are never cached. A fill in which a query failed is
// not cached: when local files supplied every failed table, what it loaded
// is served until the fill backoff elapses and the next request retries;
// otherwise the request fails with an error wrapping
// cachefill.ErrUnavailable instead of rendering with items missing.
func (s *masterdataStore) forRegion(ctx context.Context, region renderregion.Value) (*regionMasterdata, error) {
	if s == nil {
		return emptyRegionMasterdata(), nil
	}
	key := renderregion.WithDefault(region).String()
	if strings.TrimSpace(key) == "" {
		key = renderregion.CN.String()
	}

	for {
		if cached, partial := s.lookup(key); cached != nil {
			return cached, nil
		} else if partial != nil && s.fill.Failing(key) {
			return partial, nil
		}

		err := s.fill.Do(ctx, key, func(fillCtx context.Context) error {
			return s.fillRegion(fillCtx, key)
		})
		if errors.Is(err, errMasterdataFillStale) {
			continue
		}
		cached, partial := s.lookup(key)
		switch {
		case cached != nil:
			return cached, nil
		case err != nil && partial != nil:
			return partial, nil
		case err != nil:
			return nil, cachefill.Unavailable(err)
		}
	}
}

func (s *masterdataStore) fillRegion(fillCtx context.Context, key string) error {
	s.mu.RLock()
	cached := s.cache[key]
	generation := s.generation
	s.mu.RUnlock()
	if cached != nil {
		return nil
	}

	loaded, dbErr, loadErr := s.loadRegion(fillCtx, renderregion.Normalize(key))

	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return errMasterdataFillStale
	}
	if s.cache == nil {
		s.cache = make(map[string]*regionMasterdata)
	}
	if s.partial == nil {
		s.partial = make(map[string]*regionMasterdata)
	}
	if loadErr != nil {
		delete(s.partial, key)
		return loadErr
	}
	if dbErr != nil {
		s.partial[key] = loaded
		return fmt.Errorf("%w: %w", errMasterdataFillIncomplete, dbErr)
	}
	if s.cache[key] == nil {
		s.cache[key] = loaded
	}
	delete(s.partial, key)
	return nil
}

func (s *masterdataStore) lookup(key string) (cached, partial *regionMasterdata) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[key], s.partial[key]
}

func (s *masterdataStore) resetCache() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache = make(map[string]*regionMasterdata)
	s.partial = make(map[string]*regionMasterdata)
	s.generation++
	s.mu.Unlock()
	s.fill.Reset()
}

// loadRegion fills every inventory table for a region, database first. A
// table the database serves empty (or cannot serve) is read from the local
// masterdata when a local directory is configured. dbErr joins the failed
// database queries, each naming its table; err joins those whose table no
// local file supplied either, so the region cannot be served.
func (s *masterdataStore) loadRegion(fillCtx context.Context, region renderregion.Value) (md *regionMasterdata, dbErr, err error) {
	md = emptyRegionMasterdata()
	if s.client == nil && strings.TrimSpace(s.localDir) == "" {
		return md, nil, nil
	}
	if fillCtx == nil {
		fillCtx = context.Background()
	}

	tables := []struct {
		name      string
		fromDB    func(context.Context, renderregion.Value, *regionMasterdata) (bool, error)
		fromLocal func() bool
	}{
		{"materials", s.loadMaterials, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "materials.json", md.materials, func(item materialMeta) int { return item.ID })
		}},
		{"boostItems", s.loadBoostItems, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "boostItems.json", md.boostItems, func(item boostItemMeta) int { return item.ID })
		}},
		{"eventItems", s.loadEventItems, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "eventItems.json", md.eventItems, func(item eventItemMeta) int { return item.ID })
		}},
		{"gachaTickets", s.loadGachaTickets, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "gachaTickets.json", md.gachaTickets, func(item assetNamedMeta) int { return item.ID })
		}},
		{"practiceTickets", s.loadPracticeTickets, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "practiceTickets.json", md.practiceTickets, func(item ticketMeta) int { return item.ID })
		}},
		{"skillPracticeTickets", s.loadSkillPracticeTickets, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "skillPracticeTickets.json", md.skillPracticeTickets, func(item ticketMeta) int { return item.ID })
		}},
		{"gachaCeilItems", s.loadGachaCeilItems, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "gachaCeilItems.json", md.gachaCeilItems, func(item assetNamedMeta) int { return item.ID })
		}},
		{"mysekaiMaterials", s.loadMysekaiMaterials, func() bool {
			return loadIndexedMasterdata(s.localDir, region, "mysekaiMaterials.json", md.mysekaiMaterials, func(item mysekaiMaterialMeta) int { return item.ID })
		}},
	}

	var failed, unserved []error
	for _, table := range tables {
		filled, queryErr := table.fromDB(fillCtx, region, md)
		localFilled := false
		if !filled && strings.TrimSpace(s.localDir) != "" {
			localFilled = table.fromLocal()
		}
		if queryErr != nil {
			queryErr = fmt.Errorf("%s: %w", table.name, queryErr)
			failed = append(failed, queryErr)
			if !localFilled {
				unserved = append(unserved, queryErr)
			}
		}
	}
	return md, errors.Join(failed...), errors.Join(unserved...)
}

// Each loader reports filled=true when the database returned at least one
// row for the region, so the caller knows whether the local file is still
// needed.

func (s *masterdataStore) loadMaterials(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Material.Query().Where(material.ServerRegionEQ(region.String())).Order(material.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.materials[int(item.GameID)] = materialMeta{
			ID: int(item.GameID), Seq: int(item.Seq), MaterialType: item.MaterialType, Name: item.Name, FlavorText: item.FlavorText,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadBoostItems(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Boostitem.Query().Where(boostitem.ServerRegionEQ(region.String())).Order(boostitem.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.boostItems[int(item.GameID)] = boostItemMeta{
			ID: int(item.GameID), Seq: int(item.Seq), Name: item.Name, RecoveryValue: int(item.RecoveryValue), FlavorText: item.FlavorText,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadEventItems(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Eventitem.Query().Where(eventitem.ServerRegionEQ(region.String())).Order(eventitem.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.eventItems[int(item.GameID)] = eventItemMeta{
			ID: int(item.GameID), EventID: int(item.EventID), Name: item.Name, AssetbundleName: item.AssetbundleName, FlavorText: item.FlavorText,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadGachaTickets(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Gachaticket.Query().Where(gachaticket.ServerRegionEQ(region.String())).Order(gachaticket.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.gachaTickets[int(item.GameID)] = assetNamedMeta{
			ID: int(item.GameID), Name: item.Name, AssetbundleName: item.AssetbundleName,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadPracticeTickets(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Practiceticket.Query().Where(practiceticket.ServerRegionEQ(region.String())).Order(practiceticket.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.practiceTickets[int(item.GameID)] = ticketMeta{
			ID: int(item.GameID), Name: item.Name, CharacterID: int(item.CharacterID), Exp: int(item.Exp), FlavorText: item.FlavorText,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadSkillPracticeTickets(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Skillpracticeticket.Query().Where(skillpracticeticket.ServerRegionEQ(region.String())).Order(skillpracticeticket.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.skillPracticeTickets[int(item.GameID)] = ticketMeta{
			ID: int(item.GameID), Name: item.Name, CharacterID: int(item.CharacterID), Exp: int(item.Exp), FlavorText: item.FlavorText,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadGachaCeilItems(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Gachaceilitem.Query().Where(gachaceilitem.ServerRegionEQ(region.String())).Order(gachaceilitem.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.gachaCeilItems[int(item.GameID)] = assetNamedMeta{
			ID: int(item.GameID), Name: item.Name, AssetbundleName: item.AssetbundleName,
		}
	}
	return len(items) > 0, nil
}

func (s *masterdataStore) loadMysekaiMaterials(ctx context.Context, region renderregion.Value, md *regionMasterdata) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	items, err := s.client.Mysekaimaterial.Query().Where(mysekaimaterial.ServerRegionEQ(region.String())).Order(mysekaimaterial.ByGameID()).All(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.GameID <= 0 {
			continue
		}
		md.mysekaiMaterials[int(item.GameID)] = mysekaiMaterialMeta{
			ID: int(item.GameID), Seq: int(item.Seq), Type: item.MysekaiMaterialType, Name: item.Name,
			Description: item.Description, IconAssetbundleName: item.IconAssetbundleName,
		}
	}
	return len(items) > 0, nil
}

// loadIndexedMasterdata reports whether a local file for the table was
// found and decoded.
func loadIndexedMasterdata[T any](localDir string, region renderregion.Value, filename string, destination map[int]T, idOf func(T) int) bool {
	var items []T
	if loadMasterdataFile(localDir, region, filename, &items) != nil {
		return false
	}
	for _, item := range items {
		if id := idOf(item); id > 0 {
			destination[id] = item
		}
	}
	return true
}

func emptyRegionMasterdata() *regionMasterdata {
	return &regionMasterdata{
		materials:            make(map[int]materialMeta),
		boostItems:           make(map[int]boostItemMeta),
		eventItems:           make(map[int]eventItemMeta),
		gachaTickets:         make(map[int]assetNamedMeta),
		practiceTickets:      make(map[int]ticketMeta),
		skillPracticeTickets: make(map[int]ticketMeta),
		gachaCeilItems:       make(map[int]assetNamedMeta),
		mysekaiMaterials:     make(map[int]mysekaiMaterialMeta),
	}
}

func loadMasterdataFile(localDir string, region renderregion.Value, filename string, out any) error {
	for _, candidate := range masterdataFileCandidates(localDir, region, filename) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
		return nil
	}
	return os.ErrNotExist
}

func masterdataFileCandidates(localDir string, region renderregion.Value, filename string) []string {
	localDir = strings.TrimSpace(localDir)
	if localDir == "" {
		return nil
	}
	region = renderregion.WithDefault(region)
	regionKey := strings.ToLower(region.String())
	repo := masterdataRepoName(region)

	candidates := []string{
		filepath.Join(localDir, filename),
		filepath.Join(localDir, regionKey, filename),
		filepath.Join(localDir, regionKey, "master", filename),
	}
	if repo != "" {
		candidates = append(candidates,
			filepath.Join(localDir, repo, "master", filename),
			filepath.Join(localDir, repo, filename),
		)
	}
	return candidates
}

func masterdataRepoName(region renderregion.Value) string {
	switch renderregion.WithDefault(region) {
	case renderregion.JP:
		return "haruki-sekai-master"
	case renderregion.CN:
		return "haruki-sekai-sc-master"
	case renderregion.TW:
		return "haruki-sekai-tc-master"
	case renderregion.KR:
		return "haruki-sekai-kr-master"
	case renderregion.EN:
		return "haruki-sekai-en-master"
	default:
		return ""
	}
}
