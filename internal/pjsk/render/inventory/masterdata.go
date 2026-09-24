package inventory

import (
	"context"
	"errors"
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
)

// errMasterdataFillIncomplete reports a region fill in which at least one
// database query failed.
var errMasterdataFillIncomplete = errors.New("inventory masterdata fill incomplete")

func newMasterdataStore(client *sekaiDB.Client, localDir string) *masterdataStore {
	return &masterdataStore{
		client:   client,
		localDir: strings.TrimSpace(localDir),
		cache:    make(map[string]*regionMasterdata),
		partial:  make(map[string]*regionMasterdata),
	}
}

// forRegion serves a region's inventory tables, filling them once from the
// database (and local files for the tables it serves empty). The fill runs
// detached from the request context and is shared by concurrent requests.
// A fill in which a query failed is not cached: what it loaded is served
// until the fill backoff elapses, then the next request retries.
func (s *masterdataStore) forRegion(ctx context.Context, region renderregion.Value) *regionMasterdata {
	if s == nil {
		return emptyRegionMasterdata()
	}
	key := renderregion.WithDefault(region).String()
	if strings.TrimSpace(key) == "" {
		key = renderregion.CN.String()
	}

	if cached, partial := s.lookup(key); cached != nil {
		return cached
	} else if partial != nil && s.fill.Failing(key) {
		return partial
	}

	err := s.fill.Do(ctx, key, func(fillCtx context.Context) error {
		if cached, _ := s.lookup(key); cached != nil {
			return nil
		}
		loaded, complete := s.loadRegion(fillCtx, renderregion.Normalize(key))
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cache == nil {
			s.cache = make(map[string]*regionMasterdata)
		}
		if s.partial == nil {
			s.partial = make(map[string]*regionMasterdata)
		}
		if !complete {
			s.partial[key] = loaded
			return errMasterdataFillIncomplete
		}
		if s.cache[key] == nil {
			s.cache[key] = loaded
		}
		delete(s.partial, key)
		return nil
	})
	cached, partial := s.lookup(key)
	switch {
	case cached != nil:
		return cached
	case err != nil && partial != nil:
		return partial
	case err != nil:
		return emptyRegionMasterdata()
	default:
		// The cache was reset while the fill ran; fill again.
		return s.forRegion(ctx, region)
	}
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
	s.mu.Unlock()
	s.fill.Reset()
}

// loadRegion fills every inventory table for a region, database first. A
// table the database serves empty (or cannot serve) is read from the local
// masterdata when a local directory is configured. complete is false when a
// database query failed.
func (s *masterdataStore) loadRegion(fillCtx context.Context, region renderregion.Value) (*regionMasterdata, bool) {
	md := emptyRegionMasterdata()
	if s.client == nil && strings.TrimSpace(s.localDir) == "" {
		return md, true
	}
	if fillCtx == nil {
		fillCtx = context.Background()
	}

	tables := []struct {
		fromDB    func(context.Context, renderregion.Value, *regionMasterdata) (bool, error)
		fromLocal func()
	}{
		{s.loadMaterials, func() {
			loadIndexedMasterdata(s.localDir, region, "materials.json", md.materials, func(item materialMeta) int { return item.ID })
		}},
		{s.loadBoostItems, func() {
			loadIndexedMasterdata(s.localDir, region, "boostItems.json", md.boostItems, func(item boostItemMeta) int { return item.ID })
		}},
		{s.loadEventItems, func() {
			loadIndexedMasterdata(s.localDir, region, "eventItems.json", md.eventItems, func(item eventItemMeta) int { return item.ID })
		}},
		{s.loadGachaTickets, func() {
			loadIndexedMasterdata(s.localDir, region, "gachaTickets.json", md.gachaTickets, func(item assetNamedMeta) int { return item.ID })
		}},
		{s.loadPracticeTickets, func() {
			loadIndexedMasterdata(s.localDir, region, "practiceTickets.json", md.practiceTickets, func(item ticketMeta) int { return item.ID })
		}},
		{s.loadSkillPracticeTickets, func() {
			loadIndexedMasterdata(s.localDir, region, "skillPracticeTickets.json", md.skillPracticeTickets, func(item ticketMeta) int { return item.ID })
		}},
		{s.loadGachaCeilItems, func() {
			loadIndexedMasterdata(s.localDir, region, "gachaCeilItems.json", md.gachaCeilItems, func(item assetNamedMeta) int { return item.ID })
		}},
		{s.loadMysekaiMaterials, func() {
			loadIndexedMasterdata(s.localDir, region, "mysekaiMaterials.json", md.mysekaiMaterials, func(item mysekaiMaterialMeta) int { return item.ID })
		}},
	}

	complete := true
	for _, table := range tables {
		filled, err := table.fromDB(fillCtx, region, md)
		if err != nil {
			complete = false
		}
		if !filled && strings.TrimSpace(s.localDir) != "" {
			table.fromLocal()
		}
	}
	return md, complete
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

func loadIndexedMasterdata[T any](localDir string, region renderregion.Value, filename string, destination map[int]T, idOf func(T) int) {
	var items []T
	if loadMasterdataFile(localDir, region, filename, &items) != nil {
		return
	}
	for _, item := range items {
		if id := idOf(item); id > 0 {
			destination[id] = item
		}
	}
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
