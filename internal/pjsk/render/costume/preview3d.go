package costume

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"golang.org/x/sync/singleflight"
)

const defaultPreview3DStaticRelativeDir = "static_images/pjsk_3d_preview"

var preview3DImageIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Preview3DConfig struct {
	Enabled               bool
	EngineBaseURL         string
	StaticRelativeDir     string
	Width                 int
	Height                int
	Scale                 float64
	Timeout               time.Duration
	RegistryCacheTTL      time.Duration
	CaptureExistsTTL      time.Duration
	CaptureMaxConcurrency int
	CaptureAcquireTimeout time.Duration
	TemporaryCaptureTTL   time.Duration
	CaptureCacheVersion   string
	CameraPreset          string
}

type Preview3DService struct {
	cfg    Preview3DConfig
	client *http.Client

	captureFlight singleflight.Group
	captureSem    chan struct{}

	mu        sync.Mutex
	cached    *preview3DRegistry
	cachedAt  time.Time
	fetching  bool
	fetchCond *sync.Cond
	captures  map[string]time.Time
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
	Costume3DID                   int    `json:"costume3dId"`
	PartType                      string `json:"partType"`
	CharacterID                   int    `json:"characterId"`
	Unit                          string `json:"unit"`
	ColorID                       int    `json:"colorId"`
	Costume3DGroupID              int    `json:"costume3dGroupId"`
	HeadCostume3DAssetbundleType string `json:"headCostume3dAssetbundleType"`
	Status                        string `json:"status"`
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
	CharacterID             int
	Unit                    string
	Costume3DGroupID        int
	ColorID                 int
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
	if cfg.CaptureExistsTTL == 0 {
		cfg.CaptureExistsTTL = 30 * time.Second
	}
	if cfg.CaptureMaxConcurrency <= 0 {
		cfg.CaptureMaxConcurrency = 1
	}
	if cfg.CaptureAcquireTimeout <= 0 {
		cfg.CaptureAcquireTimeout = 10 * time.Second
	}
	if cfg.TemporaryCaptureTTL == 0 {
		cfg.TemporaryCaptureTTL = 15 * 24 * time.Hour
	}
	cfg.CameraPreset = normalizePreview3DCameraPreset(cfg.CameraPreset)
	service := &Preview3DService{
		cfg:        cfg,
		captureSem: make(chan struct{}, cfg.CaptureMaxConcurrency),
		captures:   make(map[string]time.Time),
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
	selection, err := registry.resolve(region, costume3DID, s.captureCacheSignature())
	if err != nil {
		return "", err
	}
	return path.Join(strings.Trim(s.cfg.StaticRelativeDir, "/"), selection.ImageID+".png"), nil
}

func (s *Preview3DService) EnsurePreviewCapture(ctx context.Context, region string, costume3DID int) error {
	if s == nil || costume3DID <= 0 {
		return nil
	}
	registry, err := s.registry(ctx)
	if err != nil {
		return err
	}
	selection, err := registry.resolve(region, costume3DID, s.captureCacheSignature())
	if err != nil {
		return err
	}
	return s.ensureCapture(ctx, selection, "persistent")
}

func (s *Preview3DService) CaptureTemporaryCombo(ctx context.Context, region string, query ComboQuery) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	registry, err := s.registry(ctx)
	if err != nil {
		return nil, err
	}
	selection, err := registry.resolveCombo(region, query, s.captureCacheSignature())
	if err != nil {
		return nil, err
	}
	if err := s.ensureCapture(ctx, selection, "temporary"); err != nil {
		return nil, err
	}
	return s.getCapture(ctx, selection.ImageID)
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
	if s.cachedCaptureExists(imageID) {
		return true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.url("/captures/"+imageID+".png"), nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if ok {
		s.markCaptureExists(imageID)
	}
	return ok
}

func (s *Preview3DService) cachedCaptureExists(imageID string) bool {
	if s == nil || s.cfg.CaptureExistsTTL < 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.captures[imageID]
	if !ok {
		return false
	}
	if time.Now().Before(expiresAt) {
		return true
	}
	delete(s.captures, imageID)
	return false
}

func (s *Preview3DService) markCaptureExists(imageID string) {
	if s == nil || s.cfg.CaptureExistsTTL < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captures[imageID] = time.Now().Add(s.cfg.CaptureExistsTTL)
}

func (s *Preview3DService) ensureCapture(ctx context.Context, selection preview3DSelection, cacheMode string) error {
	if s.captureExists(ctx, selection.ImageID) {
		return nil
	}
	_, err, _ := s.captureFlight.Do(selection.ImageID, func() (any, error) {
		if s.captureExists(ctx, selection.ImageID) {
			return nil, nil
		}
		release, err := s.acquireCapturePermit(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		if s.captureExists(ctx, selection.ImageID) {
			return nil, nil
		}
		if err := s.captureSelection(ctx, selection, cacheMode); err != nil {
			return nil, err
		}
		s.markCaptureExists(selection.ImageID)
		return nil, nil
	})
	return err
}

func (s *Preview3DService) acquireCapturePermit(ctx context.Context) (func(), error) {
	if s == nil || s.captureSem == nil {
		return func() {}, nil
	}
	waitCtx := ctx
	cancel := func() {}
	if s.cfg.CaptureAcquireTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, s.cfg.CaptureAcquireTimeout)
	}
	select {
	case s.captureSem <- struct{}{}:
		cancel()
		return func() { <-s.captureSem }, nil
	case <-waitCtx.Done():
		cancel()
		return nil, fmt.Errorf("3d preview capture is busy")
	}
}

func (s *Preview3DService) captureSelection(ctx context.Context, selection preview3DSelection, cacheMode string) error {
	if cacheMode == "" {
		cacheMode = "persistent"
	}
	body := map[string]any{
		"imageId":                 selection.ImageID,
		"roleId":                  selection.RoleID,
		"bodyCostume3dId":         selection.BodyCostume3DID,
		"headCostume3dId":         selection.HeadCostume3DID,
		"hairCostume3dId":         selection.HairCostume3DID,
		"timeoutMs":               int(s.cfg.Timeout / time.Millisecond),
		"headOptionalCostume3dId": nil,
		"cacheMode":               cacheMode,
		"cameraPreset":            s.cfg.CameraPreset,
	}
	if cacheMode == "temporary" && s.cfg.TemporaryCaptureTTL > 0 {
		body["ttlSeconds"] = int((s.cfg.TemporaryCaptureTTL + time.Second - 1) / time.Second)
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

func (s *Preview3DService) getCapture(ctx context.Context, imageID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url("/captures/"+imageID+".png"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("3d preview capture fetch returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return io.ReadAll(resp.Body)
}

func (s *Preview3DService) url(requestPath string) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.EngineBaseURL), "/")
	return base + "/" + strings.TrimLeft(requestPath, "/")
}

func (s *Preview3DService) captureCacheSignature() string {
	return preview3DCacheSignature(
		s.cfg.CaptureCacheVersion,
		s.cfg.Width,
		s.cfg.Height,
		s.cfg.Scale,
		s.cfg.CameraPreset,
	)
}

func (s *Preview3DService) CacheSignature() string {
	if s == nil {
		return ""
	}
	return s.captureCacheSignature()
}

func preview3DCacheSignature(version string, width int, height int, scale float64, cameraPreset string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "v1"
	}
	cameraPreset = normalizePreview3DCameraPreset(cameraPreset)
	material := fmt.Sprintf("%s|%d|%d|%.4f|%s", version, width, height, scale, cameraPreset)
	sum := sha256.Sum256([]byte(material))
	return "v" + hex.EncodeToString(sum[:])[:10]
}

func normalizePreview3DCameraPreset(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "default") {
		return "default"
	}
	return "capture"
}

func (r *preview3DRegistry) resolve(region string, costume3DID int, cacheSignature string) (preview3DSelection, error) {
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
	switch preview3DPartSlot(selected) {
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
	if !r.isOfficialPresetTuple(role, bodyID, headID, hairID, headOptionalID) {
		if err := r.validateHeadHairCompatibility(role.Unit, headID, hairID, "3d preview"); err != nil {
			return preview3DSelection{}, err
		}
	}

	optionalID := 0
	if headOptionalID != nil {
		optionalID = *headOptionalID
	}
	unit := sanitizePreview3DImagePart(role.Unit)
	if cacheSignature == "" {
		cacheSignature = preview3DCacheSignature("", 0, 0, 0, "")
	}
	imageID := fmt.Sprintf("pjsk3d_%s_c%d_%s_g%d_cl%d_b%d_h%d_r%d_o%d",
		cacheSignature, role.CharacterID, unit,
		selected.Costume3DGroupID, selected.ColorID, bodyID, headID, hairID, optionalID)
	return preview3DSelection{
		ImageID:                 imageID,
		RoleID:                  fmt.Sprintf("%d:%s", role.CharacterID, role.Unit),
		BodyCostume3DID:         bodyID,
		HeadCostume3DID:         headID,
		HairCostume3DID:         hairID,
		HeadOptionalCostume3DID: headOptionalID,
		CharacterID:             role.CharacterID,
		Unit:                    role.Unit,
		Costume3DGroupID:        selected.Costume3DGroupID,
		ColorID:                 selected.ColorID,
	}, nil
}

func (r *preview3DRegistry) resolveCombo(region string, query ComboQuery, cacheSignature string) (preview3DSelection, error) {
	roles := r.comboRoleCandidates(query)
	if len(roles) == 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo role not found; specify a matching unit or different part ids")
	}
	if len(roles) > 1 {
		return preview3DSelection{}, fmt.Errorf("3d combo matches multiple units; specify unit")
	}
	role := roles[0]
	anchor, ok := r.comboAnchorPart(query, role)
	if !ok {
		return preview3DSelection{}, fmt.Errorf("3d combo anchor part not found")
	}

	bodyID := role.BodyCostume3DID
	headID := role.HeadCostume3DID
	hairID := role.HairCostume3DID
	var headOptionalID *int
	for _, partType := range []string{"body", "head", "hair", "head_optional"} {
		candidate, ok := r.groupPart(anchor, role.Unit, partType)
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
	if query.BodyCostume3DID > 0 {
		part, ok := r.partForRole(query.BodyCostume3DID, role, "body")
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo body part not usable for unit=%s: %d", role.Unit, query.BodyCostume3DID)
		}
		bodyID = part.Costume3DID
	}
	if query.HairCostume3DID > 0 {
		part, ok := r.partForRole(query.HairCostume3DID, role, "hair")
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo hair part not usable for unit=%s: %d", role.Unit, query.HairCostume3DID)
		}
		hairID = part.Costume3DID
	}
	explicitHead := false
	explicitOptional := false
	for _, headAccessoryID := range query.AccessoryCostumeIDs {
		part, ok := r.partForRole(headAccessoryID, role, "head", "head_optional")
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo head/accessory part not usable for unit=%s: %d", role.Unit, headAccessoryID)
		}
		switch preview3DPartSlot(part) {
		case "head":
			if explicitHead {
				return preview3DSelection{}, fmt.Errorf("组合里多个饰品落到主饰品槽位")
			}
			explicitHead = true
			headID = part.Costume3DID
		case "head_optional":
			if explicitOptional {
				return preview3DSelection{}, fmt.Errorf("组合里多个饰品落到追加饰品槽位")
			}
			explicitOptional = true
			id := part.Costume3DID
			headOptionalID = &id
		}
	}
	if bodyID <= 0 || headID <= 0 || hairID <= 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo tuple incomplete")
	}
	if !r.isOfficialPresetTuple(role, bodyID, headID, hairID, headOptionalID) {
		if err := r.validateHeadHairCompatibility(role.Unit, headID, hairID, "3d combo"); err != nil {
			return preview3DSelection{}, err
		}
	}

	optionalID := 0
	if headOptionalID != nil {
		optionalID = *headOptionalID
	}
	if cacheSignature == "" {
		cacheSignature = preview3DCacheSignature("", 0, 0, 0, "")
	}
	unit := sanitizePreview3DImagePart(role.Unit)
	imageID := fmt.Sprintf("tmp_pjsk3d_%s_combo_c%d_%s_b%d_h%d_r%d_o%d",
		cacheSignature, role.CharacterID, unit, bodyID, headID, hairID, optionalID)
	return preview3DSelection{
		ImageID:                 imageID,
		RoleID:                  fmt.Sprintf("%d:%s", role.CharacterID, role.Unit),
		BodyCostume3DID:         bodyID,
		HeadCostume3DID:         headID,
		HairCostume3DID:         hairID,
		HeadOptionalCostume3DID: headOptionalID,
		CharacterID:             role.CharacterID,
		Unit:                    role.Unit,
		Costume3DGroupID:        anchor.Costume3DGroupID,
		ColorID:                 anchor.ColorID,
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

func (r *preview3DRegistry) comboRoleCandidates(query ComboQuery) []preview3DCharacterEntry {
	var candidates []preview3DCharacterEntry
	seen := make(map[string]struct{})
	unit := strings.TrimSpace(query.Unit)
	for _, role := range r.characters {
		if !preview3DStatusUsable(role.Status) {
			continue
		}
		if unit != "" && role.Unit != unit {
			continue
		}
		if !r.comboRoleMatches(query, role) {
			continue
		}
		key := fmt.Sprintf("%d:%s", role.CharacterID, role.Unit)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, role)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CharacterID == candidates[j].CharacterID {
			return candidates[i].Unit < candidates[j].Unit
		}
		return candidates[i].CharacterID < candidates[j].CharacterID
	})
	return candidates
}

func (r *preview3DRegistry) comboRoleMatches(query ComboQuery, role preview3DCharacterEntry) bool {
	if query.BodyCostume3DID > 0 {
		if _, ok := r.partForRole(query.BodyCostume3DID, role, "body"); !ok {
			return false
		}
	}
	if query.HairCostume3DID > 0 {
		if _, ok := r.partForRole(query.HairCostume3DID, role, "hair"); !ok {
			return false
		}
	}
	for _, headAccessoryID := range query.AccessoryCostumeIDs {
		if _, ok := r.partForRole(headAccessoryID, role, "head", "head_optional"); !ok {
			return false
		}
	}
	return true
}

func (r *preview3DRegistry) comboAnchorPart(query ComboQuery, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	if query.BodyCostume3DID > 0 {
		return r.partForRole(query.BodyCostume3DID, role, "body")
	}
	if query.HairCostume3DID > 0 {
		return r.partForRole(query.HairCostume3DID, role, "hair")
	}
	for _, headAccessoryID := range query.AccessoryCostumeIDs {
		if part, ok := r.partForRole(headAccessoryID, role, "head", "head_optional"); ok {
			return part, true
		}
	}
	return preview3DPartEntry{}, false
}

func (r *preview3DRegistry) partForRole(costume3DID int, role preview3DCharacterEntry, partTypes ...string) (preview3DPartEntry, bool) {
	allowed := make(map[string]struct{}, len(partTypes))
	for _, partType := range partTypes {
		allowed[normalizePreview3DPartType(partType)] = struct{}{}
	}
	for _, part := range r.parts {
		if part.Costume3DID != costume3DID || !preview3DStatusUsable(part.Status) {
			continue
		}
		if part.CharacterID != role.CharacterID {
			continue
		}
		if part.Unit != "" && part.Unit != role.Unit {
			continue
		}
		if _, ok := allowed[preview3DPartSlot(part)]; !ok {
			continue
		}
		return part, true
	}
	return preview3DPartEntry{}, false
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
		if preview3DPartSlot(part) != partType {
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

func (r *preview3DRegistry) isOfficialPresetTuple(role preview3DCharacterEntry, bodyID int, headID int, hairID int, headOptionalID *int) bool {
	if headOptionalID != nil {
		return false
	}
	for _, candidate := range r.characters {
		if !preview3DStatusUsable(candidate.Status) {
			continue
		}
		if candidate.CharacterID != role.CharacterID || candidate.Unit != role.Unit {
			continue
		}
		if candidate.BodyCostume3DID == bodyID &&
			candidate.HeadCostume3DID == headID &&
			candidate.HairCostume3DID == hairID {
			return true
		}
	}
	return false
}

func (r *preview3DRegistry) validateHeadHairCompatibility(unit string, headID int, hairID int, label string) error {
	for _, rule := range r.rules {
		if rule.HeadCostume3DID != headID {
			continue
		}
		if unit != "" && rule.Unit != "" && rule.Unit != unit {
			continue
		}
		if rule.HairCostume3DID == hairID && strings.EqualFold(rule.State, "not_available") {
			return fmt.Errorf("%s head/hair combination is blocked: unit=%s head=%d hair=%d", label, unit, headID, hairID)
		}
	}
	return nil
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

func preview3DPartSlot(part preview3DPartEntry) string {
	slot := normalizePreview3DPartType(part.PartType)
	if slot == "head" || slot == "head_optional" {
		switch strings.TrimSpace(strings.ToLower(part.HeadCostume3DAssetbundleType)) {
		case "head_and_hair", "head_all", "head_front", "head_back":
			return "head"
		}
	}
	return slot
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
