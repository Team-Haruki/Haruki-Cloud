package inventory

import (
	"os"
	"path/filepath"
	"strings"

	json "haruki-cloud/internal/jsonutil"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func newMasterdataStore(localDir string) *masterdataStore {
	return &masterdataStore{
		localDir: strings.TrimSpace(localDir),
		cache:    make(map[string]*regionMasterdata),
	}
}

func (s *masterdataStore) forRegion(region renderregion.Value) *regionMasterdata {
	if s == nil {
		return emptyRegionMasterdata()
	}
	key := renderregion.WithDefault(region).String()
	if strings.TrimSpace(key) == "" {
		key = renderregion.CN.String()
	}

	s.mu.RLock()
	cached := s.cache[key]
	s.mu.RUnlock()
	if cached != nil {
		return cached
	}

	loaded := s.loadRegion(renderregion.Normalize(key))
	s.mu.Lock()
	if existing := s.cache[key]; existing != nil {
		s.mu.Unlock()
		return existing
	}
	s.cache[key] = loaded
	s.mu.Unlock()
	return loaded
}

func (s *masterdataStore) resetCache() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache = make(map[string]*regionMasterdata)
	s.mu.Unlock()
}

func (s *masterdataStore) loadRegion(region renderregion.Value) *regionMasterdata {
	md := emptyRegionMasterdata()
	if strings.TrimSpace(s.localDir) == "" {
		return md
	}

	var materials []materialMeta
	if loadMasterdataFile(s.localDir, region, "materials.json", &materials) == nil {
		for _, item := range materials {
			if item.ID > 0 {
				md.materials[item.ID] = item
			}
		}
	}

	var boostItems []boostItemMeta
	if loadMasterdataFile(s.localDir, region, "boostItems.json", &boostItems) == nil {
		for _, item := range boostItems {
			if item.ID > 0 {
				md.boostItems[item.ID] = item
			}
		}
	}

	var eventItems []eventItemMeta
	if loadMasterdataFile(s.localDir, region, "eventItems.json", &eventItems) == nil {
		for _, item := range eventItems {
			if item.ID > 0 {
				md.eventItems[item.ID] = item
			}
		}
	}

	var gachaTickets []assetNamedMeta
	if loadMasterdataFile(s.localDir, region, "gachaTickets.json", &gachaTickets) == nil {
		for _, item := range gachaTickets {
			if item.ID > 0 {
				md.gachaTickets[item.ID] = item
			}
		}
	}

	var practiceTickets []ticketMeta
	if loadMasterdataFile(s.localDir, region, "practiceTickets.json", &practiceTickets) == nil {
		for _, item := range practiceTickets {
			if item.ID > 0 {
				md.practiceTickets[item.ID] = item
			}
		}
	}

	var skillPracticeTickets []ticketMeta
	if loadMasterdataFile(s.localDir, region, "skillPracticeTickets.json", &skillPracticeTickets) == nil {
		for _, item := range skillPracticeTickets {
			if item.ID > 0 {
				md.skillPracticeTickets[item.ID] = item
			}
		}
	}

	var gachaCeilItems []assetNamedMeta
	if loadMasterdataFile(s.localDir, region, "gachaCeilItems.json", &gachaCeilItems) == nil {
		for _, item := range gachaCeilItems {
			if item.ID > 0 {
				md.gachaCeilItems[item.ID] = item
			}
		}
	}

	var mysekaiMaterials []mysekaiMaterialMeta
	if loadMasterdataFile(s.localDir, region, "mysekaiMaterials.json", &mysekaiMaterials) == nil {
		for _, item := range mysekaiMaterials {
			if item.ID > 0 {
				md.mysekaiMaterials[item.ID] = item
			}
		}
	}
	return md
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
