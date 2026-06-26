package costume

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultPreview3DStaticRelativeDir = "static_images/pjsk_3d_preview"

var preview3DImageIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Preview3DConfig struct {
	Enabled           bool
	EngineBaseURL     string
	StaticRelativeDir string
	Width             int
	Height            int
	Scale             float64
	Timeout           time.Duration
	RegistryCacheTTL  time.Duration
}

type Preview3DService struct {
	cfg    Preview3DConfig
	client *http.Client

	mu        sync.Mutex
	cached    *preview3DRegistry
	cachedAt  time.Time
	fetching  bool
	fetchCond *sync.Cond
}

type preview3DRegistry struct {
	characters []preview3DCharacterEntry
	parts      []preview3DPartEntry
	rules      []preview3DCompatibilityRule
}

type preview3DCharacterIndex struct {
	Entries []preview3DCharacterEntry `json:"entries"`
}

type preview3DCharacterEntry struct {
	Character3DID   int    `json:"character3dId"`
	CharacterID     int    `json:"characterId"`
	Unit            string `json:"unit"`
	BodyCostume3DID int    `json:"bodyCostume3dId"`
	HeadCostume3DID int    `json:"headCostume3dId"`
	HairCostume3DID int    `json:"hairCostume3dId"`
	Status          string `json:"status"`
}

type preview3DPartRegistry struct {
	Entries []preview3DPartEntry `json:"entries"`
}

type preview3DPartEntry struct {
	Costume3DID      int    `json:"costume3dId"`
	PartType         string `json:"partType"`
	CharacterID      int    `json:"characterId"`
	Unit             string `json:"unit"`
	ColorID          int    `json:"colorId"`
	Costume3DGroupID int    `json:"costume3dGroupId"`
	Status           string `json:"status"`
}

type preview3DCompatibilityRegistry struct {
	Rules []preview3DCompatibilityRule `json:"rules"`
}

type preview3DCompatibilityRule struct {
	Unit            string `json:"unit"`
	HeadCostume3DID int    `json:"headCostume3dId"`
	HairCostume3DID int    `json:"hairCostume3dId"`
	State           string `json:"state"`
}

type preview3DSelection struct {
	ImageID                 string
	RoleID                  string
	BodyCostume3DID         int
	HeadCostume3DID         int
	HairCostume3DID         int
	HeadOptionalCostume3DID *int
}

func NewPreview3DService(cfg Preview3DConfig) *Preview3DService {
	if !cfg.Enabled || strings.TrimSpace(cfg.EngineBaseURL) == "" {
		return nil
	}
	if cfg.StaticRelativeDir == "" {
		cfg.StaticRelativeDir = defaultPreview3DStaticRelativeDir
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.RegistryCacheTTL == 0 {
		cfg.RegistryCacheTTL = 5 * time.Minute
	}
	service := &Preview3DService{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
	service.fetchCond = sync.NewCond(&service.mu)
	return service
}

func (s *Preview3DService) ResolvePreviewPath(ctx context.Context, region string, costume3DID int) (string, error) {
	if s == nil || costume3DID <= 0 {
		return "", nil
	}
	registry, err := s.registry(ctx)
	if err != nil {
		return "", err
	}
	selection, err := registry.resolve(region, costume3DID)
	if err != nil {
		return "", err
	}
	previewPath := path.Join(strings.Trim(s.cfg.StaticRelativeDir, "/"), selection.ImageID+".png")
	if s.captureExists(ctx, selection.ImageID) {
		return previewPath, nil
	}
	if err := s.capture(ctx, selection); err != nil {
		return "", err
	}
	return previewPath, nil
}

func (s *Preview3DService) registry(ctx context.Context) (*preview3DRegistry, error) {
	if cached := s.validCachedRegistry(); cached != nil {
		return cached, nil
	}

	s.mu.Lock()
	for s.fetching {
		s.fetchCond.Wait()
	}
	if cached := s.validCachedRegistryLocked(time.Now()); cached != nil {
		s.mu.Unlock()
		return cached, nil
	}
	s.fetching = true
	s.mu.Unlock()

	registry, err := s.fetchRegistry(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetching = false
	if err == nil {
		s.cached = registry
		s.cachedAt = time.Now()
	}
	s.fetchCond.Broadcast()
	return registry, err
}

func (s *Preview3DService) validCachedRegistry() *preview3DRegistry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validCachedRegistryLocked(time.Now())
}

func (s *Preview3DService) validCachedRegistryLocked(now time.Time) *preview3DRegistry {
	if s.cached == nil {
		return nil
	}
	if s.cfg.RegistryCacheTTL < 0 || now.Sub(s.cachedAt) <= s.cfg.RegistryCacheTTL {
		return s.cached
	}
	return nil
}

func (s *Preview3DService) fetchRegistry(ctx context.Context) (*preview3DRegistry, error) {
	var characterIndex preview3DCharacterIndex
	if err := s.getJSON(ctx, "/runtime/character3d-index.json", &characterIndex); err != nil {
		return nil, err
	}
	var partRegistry preview3DPartRegistry
	if err := s.getJSON(ctx, "/runtime/parts/part-registry.json", &partRegistry); err != nil {
		return nil, err
	}
	var compatibility preview3DCompatibilityRegistry
	if err := s.getJSON(ctx, "/runtime/parts/head-hair-compatibility.json", &compatibility); err != nil {
		return nil, err
	}
	return &preview3DRegistry{
		characters: characterIndex.Entries,
		parts:      partRegistry.Entries,
		rules:      compatibility.Rules,
	}, nil
}

func (s *Preview3DService) getJSON(ctx context.Context, requestPath string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(requestPath), nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("3d preview registry %s returned HTTP %d", requestPath, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Preview3DService) captureExists(ctx context.Context, imageID string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.url("/captures/"+imageID+".png"), nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (s *Preview3DService) capture(ctx context.Context, selection preview3DSelection) error {
	body := map[string]any{
		"imageId":                 selection.ImageID,
		"roleId":                  selection.RoleID,
		"bodyCostume3dId":         selection.BodyCostume3DID,
		"headCostume3dId":         selection.HeadCostume3DID,
		"hairCostume3dId":         selection.HairCostume3DID,
		"timeoutMs":               int(s.cfg.Timeout / time.Millisecond),
		"headOptionalCostume3dId": nil,
	}
	if selection.HeadOptionalCostume3DID != nil {
		body["headOptionalCostume3dId"] = *selection.HeadOptionalCostume3DID
	}
	if s.cfg.Width > 0 {
		body["width"] = s.cfg.Width
	}
	if s.cfg.Height > 0 {
		body["height"] = s.cfg.Height
	}
	if s.cfg.Scale > 0 {
		body["scale"] = s.cfg.Scale
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url("/capture"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("3d preview capture returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func (s *Preview3DService) url(requestPath string) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.EngineBaseURL), "/")
	return base + "/" + strings.TrimLeft(requestPath, "/")
}

func (r *preview3DRegistry) resolve(region string, costume3DID int) (preview3DSelection, error) {
	selected, ok := r.partByID(costume3DID)
	if !ok {
		return preview3DSelection{}, fmt.Errorf("3d preview part not found: %d", costume3DID)
	}
	role := r.defaultRoleForPart(selected)
	if role.CharacterID == 0 {
		return preview3DSelection{}, fmt.Errorf("3d preview default role not found for character %d", selected.CharacterID)
	}

	bodyID := role.BodyCostume3DID
	headID := role.HeadCostume3DID
	hairID := role.HairCostume3DID
	var headOptionalID *int
	for _, partType := range []string{"body", "head", "hair", "head_optional"} {
		candidate, ok := r.groupPart(selected, role.Unit, partType)
		if !ok {
			continue
		}
		switch partType {
		case "body":
			bodyID = candidate.Costume3DID
		case "head":
			headID = candidate.Costume3DID
		case "hair":
			hairID = candidate.Costume3DID
		case "head_optional":
			id := candidate.Costume3DID
			headOptionalID = &id
		}
	}
	switch normalizePreview3DPartType(selected.PartType) {
	case "body":
		bodyID = selected.Costume3DID
	case "head":
		headID = selected.Costume3DID
	case "hair":
		hairID = selected.Costume3DID
	case "head_optional":
		id := selected.Costume3DID
		headOptionalID = &id
	}
	if bodyID <= 0 || headID <= 0 || hairID <= 0 {
		return preview3DSelection{}, fmt.Errorf("3d preview tuple incomplete for costume %d", costume3DID)
	}
	if r.isHeadHairBlocked(role.Unit, headID, hairID) {
		return preview3DSelection{}, fmt.Errorf("3d preview head/hair combination is blocked: unit=%s head=%d hair=%d", role.Unit, headID, hairID)
	}

	optionalID := 0
	if headOptionalID != nil {
		optionalID = *headOptionalID
	}
	unit := sanitizePreview3DImagePart(role.Unit)
	imageID := fmt.Sprintf("pjsk3d_%s_c%d_%s_i%d_b%d_h%d_r%d_o%d",
		sanitizePreview3DImagePart(region), role.CharacterID, unit, costume3DID, bodyID, headID, hairID, optionalID)
	return preview3DSelection{
		ImageID:                 imageID,
		RoleID:                  fmt.Sprintf("%d:%s", role.CharacterID, role.Unit),
		BodyCostume3DID:         bodyID,
		HeadCostume3DID:         headID,
		HairCostume3DID:         hairID,
		HeadOptionalCostume3DID: headOptionalID,
	}, nil
}

func (r *preview3DRegistry) partByID(costume3DID int) (preview3DPartEntry, bool) {
	for _, part := range r.parts {
		if part.Costume3DID == costume3DID && preview3DStatusUsable(part.Status) {
			return part, true
		}
	}
	return preview3DPartEntry{}, false
}

func (r *preview3DRegistry) defaultRoleForPart(part preview3DPartEntry) preview3DCharacterEntry {
	var candidates []preview3DCharacterEntry
	for _, role := range r.characters {
		if !preview3DStatusUsable(role.Status) {
			continue
		}
		if role.CharacterID != part.CharacterID {
			continue
		}
		if part.Unit != "" && role.Unit != part.Unit {
			continue
		}
		candidates = append(candidates, role)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Character3DID < candidates[j].Character3DID
	})
	if len(candidates) == 0 {
		return preview3DCharacterEntry{}
	}
	return candidates[0]
}

func (r *preview3DRegistry) groupPart(selected preview3DPartEntry, unit string, partType string) (preview3DPartEntry, bool) {
	var candidates []preview3DPartEntry
	for _, part := range r.parts {
		if !preview3DStatusUsable(part.Status) {
			continue
		}
		if part.CharacterID != selected.CharacterID || part.Costume3DGroupID != selected.Costume3DGroupID {
			continue
		}
		if unit != "" && part.Unit != "" && part.Unit != unit {
			continue
		}
		if normalizePreview3DPartType(part.PartType) != partType {
			continue
		}
		candidates = append(candidates, part)
	}
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftRank := preview3DColorRank(candidates[i].ColorID, selected.ColorID)
		rightRank := preview3DColorRank(candidates[j].ColorID, selected.ColorID)
		if leftRank == rightRank {
			return candidates[i].Costume3DID < candidates[j].Costume3DID
		}
		return leftRank < rightRank
	})
	return candidates[0], true
}

func preview3DStatusUsable(status string) bool {
	return !strings.EqualFold(strings.TrimSpace(status), "missing")
}

func (r *preview3DRegistry) isHeadHairBlocked(unit string, headID int, hairID int) bool {
	for _, rule := range r.rules {
		if rule.HeadCostume3DID != headID || rule.HairCostume3DID != hairID {
			continue
		}
		if unit != "" && rule.Unit != "" && rule.Unit != unit {
			continue
		}
		return strings.EqualFold(rule.State, "not_available")
	}
	return false
}

func preview3DColorRank(colorID int, selectedColorID int) int {
	switch colorID {
	case selectedColorID:
		return 0
	case 1:
		return 1
	default:
		return 1000 + colorID
	}
}

func normalizePreview3DPartType(partType string) string {
	partType = strings.TrimSpace(strings.ToLower(partType))
	switch partType {
	case "accessory", "option", "optional", "headoptional", "head_optional":
		return "head_optional"
	default:
		return partType
	}
}

func sanitizePreview3DImagePart(value string) string {
	value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), "._-")
	if value == "" {
		return "unknown"
	}
	value = preview3DImageIDUnsafe.ReplaceAllString(value, "_")
	value = strings.Trim(value, "._-")
	if value == "" {
		return "unknown"
	}
	return value
}
