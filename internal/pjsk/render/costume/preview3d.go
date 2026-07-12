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
	"os"
	"path"
	"path/filepath"
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
	EngineBaseURLs        map[string]string
	StaticRelativeDir     string
	StaticOutputDir       string
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
	cached    map[string]*preview3DRegistry
	cachedAt  map[string]time.Time
	fetching  map[string]bool
	fetchCond *sync.Cond
	captures  map[string]time.Time
}

type preview3DEndpoint struct {
	region  string
	baseURL string
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
	Costume3DID                  int      `json:"costume3dId"`
	PartType                     string   `json:"partType"`
	CharacterID                  int      `json:"characterId"`
	Unit                         string   `json:"unit"`
	ColorID                      int      `json:"colorId"`
	Costume3DGroupID             int      `json:"costume3dGroupId"`
	OutfitID                     int      `json:"outfitId"`
	AccessoryID                  int      `json:"accessoryId"`
	BundlePath                   string   `json:"bundlePath"`
	ColorVariationBundlePath     string   `json:"colorVariationBundlePath"`
	HeadCostume3DAssetbundleType string   `json:"headCostume3dAssetbundleType"`
	PackagePath                  string   `json:"packagePath"`
	Status                       string   `json:"status"`
	Warnings                     []string `json:"warnings"`
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
	cfg.EngineBaseURLs = normalizePreview3DEngineBaseURLs(cfg.EngineBaseURLs)
	if !cfg.Enabled || (strings.TrimSpace(cfg.EngineBaseURL) == "" && len(cfg.EngineBaseURLs) == 0) {
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
		cached:     make(map[string]*preview3DRegistry),
		cachedAt:   make(map[string]time.Time),
		fetching:   make(map[string]bool),
		captures:   make(map[string]time.Time),
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
	service.fetchCond = sync.NewCond(&service.mu)
	return service
}

func (s *Preview3DService) ResolvePreviewPath(ctx context.Context, region string, costume3DID int) (string, error) {
	return s.ResolveQueryPreviewPath(ctx, region, costume3DID, Query{})
}

func (s *Preview3DService) ResolveQueryPreviewPath(ctx context.Context, region string, costume3DID int, query Query) (string, error) {
	if s == nil || costume3DID <= 0 {
		return "", nil
	}
	endpoint, err := s.endpointForRegion(region)
	if err != nil {
		return "", err
	}
	registry, err := s.registry(ctx, endpoint)
	if err != nil {
		return "", err
	}
	selection, err := registry.resolveQuery(region, costume3DID, query, s.captureCacheSignature())
	if err != nil {
		return "", err
	}
	return path.Join(strings.Trim(s.cfg.StaticRelativeDir, "/"), selection.ImageID+".png"), nil
}

func (s *Preview3DService) EnsurePreviewCapture(ctx context.Context, region string, costume3DID int) error {
	return s.EnsureQueryPreviewCapture(ctx, region, costume3DID, Query{})
}

func (s *Preview3DService) EnsureQueryPreviewCapture(ctx context.Context, region string, costume3DID int, query Query) error {
	if s == nil || costume3DID <= 0 {
		return nil
	}
	endpoint, err := s.endpointForRegion(region)
	if err != nil {
		return err
	}
	registry, err := s.registry(ctx, endpoint)
	if err != nil {
		return err
	}
	selection, err := registry.resolveQuery(region, costume3DID, query, s.captureCacheSignature())
	if err != nil {
		return err
	}
	if err := s.ensureCapture(ctx, endpoint, selection, "persistent"); err != nil {
		return err
	}
	return s.ensureStaticCaptureFile(ctx, endpoint, selection.ImageID)
}

func (s *Preview3DService) CaptureTemporaryCombo(ctx context.Context, region string, query ComboQuery) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	endpoint, err := s.endpointForRegion(region)
	if err != nil {
		return nil, err
	}
	registry, err := s.registry(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	selection, err := registry.resolveCombo(region, query, s.captureCacheSignature())
	if err != nil {
		return nil, err
	}
	if err := s.ensureCapture(ctx, endpoint, selection, "temporary"); err != nil {
		return nil, err
	}
	return s.getCapture(ctx, endpoint, selection.ImageID)
}

func (s *Preview3DService) registry(ctx context.Context, endpoint preview3DEndpoint) (*preview3DRegistry, error) {
	if cached := s.validCachedRegistry(endpoint); cached != nil {
		return cached, nil
	}

	s.mu.Lock()
	key := endpoint.key()
	for s.fetching[key] {
		s.fetchCond.Wait()
	}
	if cached := s.validCachedRegistryLocked(endpoint, time.Now()); cached != nil {
		s.mu.Unlock()
		return cached, nil
	}
	s.fetching[key] = true
	s.mu.Unlock()

	registry, err := s.fetchRegistry(ctx, endpoint)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetching[key] = false
	if err == nil {
		s.cached[key] = registry
		s.cachedAt[key] = time.Now()
	}
	s.fetchCond.Broadcast()
	return registry, err
}

func (s *Preview3DService) validCachedRegistry(endpoint preview3DEndpoint) *preview3DRegistry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validCachedRegistryLocked(endpoint, time.Now())
}

func (s *Preview3DService) validCachedRegistryLocked(endpoint preview3DEndpoint, now time.Time) *preview3DRegistry {
	key := endpoint.key()
	cached := s.cached[key]
	if cached == nil {
		return nil
	}
	if s.cfg.RegistryCacheTTL < 0 || now.Sub(s.cachedAt[key]) <= s.cfg.RegistryCacheTTL {
		return cached
	}
	return nil
}

func (s *Preview3DService) fetchRegistry(ctx context.Context, endpoint preview3DEndpoint) (*preview3DRegistry, error) {
	var characterIndex preview3DCharacterIndex
	if err := s.getJSON(ctx, endpoint, "/runtime/character3d-index.json", &characterIndex); err != nil {
		return nil, err
	}
	var partRegistry preview3DPartRegistry
	if err := s.getJSON(ctx, endpoint, "/runtime/parts/part-registry.json", &partRegistry); err != nil {
		return nil, err
	}
	var compatibility preview3DCompatibilityRegistry
	if err := s.getJSON(ctx, endpoint, "/runtime/parts/head-hair-compatibility.json", &compatibility); err != nil {
		return nil, err
	}
	return &preview3DRegistry{
		characters: characterIndex.Entries,
		parts:      partRegistry.Entries,
		rules:      compatibility.Rules,
	}, nil
}

func (s *Preview3DService) getJSON(ctx context.Context, endpoint preview3DEndpoint, requestPath string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(endpoint, requestPath), nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("3d preview registry request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("3d preview registry %s returned HTTP %d", requestPath, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Preview3DService) captureExists(ctx context.Context, endpoint preview3DEndpoint, imageID string) bool {
	if s.cachedCaptureExists(endpoint, imageID) {
		return true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.url(endpoint, "/captures/"+imageID+".png"), nil)
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
		s.markCaptureExists(endpoint, imageID)
	}
	return ok
}

func (s *Preview3DService) cachedCaptureExists(endpoint preview3DEndpoint, imageID string) bool {
	if s == nil || s.cfg.CaptureExistsTTL < 0 {
		return false
	}
	key := endpoint.captureKey(imageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.captures[key]
	if !ok {
		return false
	}
	if time.Now().Before(expiresAt) {
		return true
	}
	delete(s.captures, key)
	return false
}

func (s *Preview3DService) markCaptureExists(endpoint preview3DEndpoint, imageID string) {
	if s == nil || s.cfg.CaptureExistsTTL < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captures[endpoint.captureKey(imageID)] = time.Now().Add(s.cfg.CaptureExistsTTL)
}

func (s *Preview3DService) ensureCapture(ctx context.Context, endpoint preview3DEndpoint, selection preview3DSelection, cacheMode string) error {
	if s.captureExists(ctx, endpoint, selection.ImageID) {
		return nil
	}
	_, err, _ := s.captureFlight.Do(endpoint.captureKey(selection.ImageID), func() (any, error) {
		if s.captureExists(ctx, endpoint, selection.ImageID) {
			return nil, nil
		}
		release, err := s.acquireCapturePermit(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		if s.captureExists(ctx, endpoint, selection.ImageID) {
			return nil, nil
		}
		if err := s.captureSelection(ctx, endpoint, selection, cacheMode); err != nil {
			return nil, err
		}
		s.markCaptureExists(endpoint, selection.ImageID)
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

func (s *Preview3DService) captureSelection(ctx context.Context, endpoint preview3DEndpoint, selection preview3DSelection, cacheMode string) error {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url(endpoint, "/capture"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("3d preview capture request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("3d preview capture returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func (s *Preview3DService) getCapture(ctx context.Context, endpoint preview3DEndpoint, imageID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(endpoint, "/captures/"+imageID+".png"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("3d preview capture fetch request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("3d preview capture fetch returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return io.ReadAll(resp.Body)
}

func (s *Preview3DService) ensureStaticCaptureFile(ctx context.Context, endpoint preview3DEndpoint, imageID string) error {
	staticOutputDir := strings.TrimSpace(s.cfg.StaticOutputDir)
	if staticOutputDir == "" || strings.TrimSpace(imageID) == "" {
		return nil
	}
	target := filepath.Join(staticOutputDir, imageID+".png")
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	data, err := s.getCapture(ctx, endpoint, imageID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func (s *Preview3DService) url(endpoint preview3DEndpoint, requestPath string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint.baseURL), "/")
	return base + "/" + strings.TrimLeft(requestPath, "/")
}

func (s *Preview3DService) endpointForRegion(region string) (preview3DEndpoint, error) {
	normalized := normalizePreview3DRegion(region)
	baseURL := ""
	if len(s.cfg.EngineBaseURLs) > 0 {
		baseURL = strings.TrimSpace(s.cfg.EngineBaseURLs[normalized])
	} else {
		baseURL = strings.TrimSpace(s.cfg.EngineBaseURL)
	}
	if baseURL == "" {
		return preview3DEndpoint{}, fmt.Errorf("3d preview engine is not configured for region %s", normalized)
	}
	return preview3DEndpoint{region: normalized, baseURL: baseURL}, nil
}

func (e preview3DEndpoint) key() string {
	return e.region + "|" + strings.TrimRight(strings.TrimSpace(e.baseURL), "/")
}

func (e preview3DEndpoint) captureKey(imageID string) string {
	return e.key() + "|" + imageID
}

func normalizePreview3DRegion(region string) string {
	normalized := strings.ToLower(strings.TrimSpace(region))
	if normalized == "" {
		return "jp"
	}
	return sanitizePreview3DImagePart(normalized)
}

func normalizePreview3DEngineBaseURLs(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(values))
	for region, baseURL := range values {
		region = normalizePreview3DRegion(region)
		baseURL = strings.TrimSpace(baseURL)
		if region == "" {
			continue
		}
		normalized[region] = baseURL
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
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

func (r *preview3DRegistry) resolve(region string, costume3DID int) (preview3DSelection, error) {
	selected, ok := r.partByID(costume3DID)
	if !ok {
		if missing, found := r.anyPartByID(costume3DID); found {
			return preview3DSelection{}, fmt.Errorf("3d preview part is missing runtime package: %s", preview3DPartDiagnostic(missing))
		}
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
			headID = candidate.Costume3DID
			headOptionalID = nil
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
		headID = selected.Costume3DID
		headOptionalID = nil
	}
	if bodyID <= 0 || headID <= 0 || hairID <= 0 {
		return preview3DSelection{}, fmt.Errorf("3d preview tuple incomplete for costume %d", costume3DID)
	}
	if slot := preview3DPartSlot(selected); slot != "head" && slot != "head_optional" {
		if officialHeadID, ok := r.officialHeadForRoleTuple(role, bodyID, hairID); ok {
			headID = officialHeadID
			if preview3DPartSlot(selected) != "head_optional" {
				headOptionalID = nil
			}
		}
	}
	if !r.isOfficialPresetTuple(role, bodyID, headID, hairID, headOptionalID) {
		var err error
		headID, hairID, err = r.applyHeadHairFallback(role, preview3DPartSlot(selected), headID, hairID, "3d preview")
		if err != nil {
			return preview3DSelection{}, err
		}
	}

	optionalID := 0
	if headOptionalID != nil {
		optionalID = *headOptionalID
	}
	unit := sanitizePreview3DImagePart(role.Unit)
	region = normalizePreview3DRegion(region)
	imageID := fmt.Sprintf("pjsk3d_%s_c%d_%s_i%d_b%d_h%d_r%d_o%d",
		region, role.CharacterID, unit, costume3DID, bodyID, headID, hairID, optionalID)
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

func (r *preview3DRegistry) resolveQuery(region string, costume3DID int, query Query, cacheSignature string) (preview3DSelection, error) {
	if query.Character3DID > 0 && (query.OutfitID > 0 || query.AccessoryID > 0) {
		return r.resolveCombo(region, ComboQuery{
			Region:           region,
			OutfitID:         query.OutfitID,
			OutfitColorID:    query.ColorID,
			AccessoryID:      query.AccessoryID,
			AccessoryColorID: query.ColorID,
			Character3DID:    query.Character3DID,
		}, cacheSignature)
	}
	return r.resolve(region, costume3DID)
}

func (r *preview3DRegistry) resolveCombo(region string, query ComboQuery, cacheSignature string) (preview3DSelection, error) {
	roles := r.comboRoleCandidates(query)
	if len(roles) == 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo role not found: character3d=%d", query.Character3DID)
	}
	if len(roles) > 1 {
		return preview3DSelection{}, fmt.Errorf("3d combo character3d id is duplicated: %d", query.Character3DID)
	}
	role := roles[0]
	anchor, ok := r.comboAnchorPart(query, role)
	if !ok {
		if query.OutfitID > 0 {
			return preview3DSelection{}, fmt.Errorf("3d combo outfit not usable: outfit=%d character3d=%d color=%d", query.OutfitID, query.Character3DID, query.OutfitColorID)
		}
		if query.AccessoryID > 0 {
			return preview3DSelection{}, fmt.Errorf("3d combo accessory not usable: accessory=%d character3d=%d color=%d", query.AccessoryID, query.Character3DID, query.AccessoryColorID)
		}
		return preview3DSelection{}, fmt.Errorf("3d combo anchor part not found")
	}

	bodyID := role.BodyCostume3DID
	headID := role.HeadCostume3DID
	hairID := role.HairCostume3DID
	var headOptionalID *int
	if query.OutfitID > 0 {
		part, ok := r.outfitPartForRole(query.OutfitID, query.OutfitColorID, role)
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo outfit not usable: outfit=%d character3d=%d color=%d", query.OutfitID, query.Character3DID, query.OutfitColorID)
		}
		bodyID = part.Costume3DID
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
	explicitHair := query.HairCostume3DID > 0
	if query.AccessoryID > 0 {
		part, ok := r.accessoryPartForRole(query.AccessoryID, query.AccessoryColorID, role)
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo accessory not usable: accessory=%d character3d=%d color=%d", query.AccessoryID, query.Character3DID, query.AccessoryColorID)
		}
		explicitHead = true
		headID = part.Costume3DID
		headOptionalID = nil
	} else if query.AccessoryCostume3DID > 0 {
		part, ok := r.partForRole(query.AccessoryCostume3DID, role, "head", "head_optional")
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo head/accessory part not usable for unit=%s: %d", role.Unit, query.AccessoryCostume3DID)
		}
		explicitHead = true
		headID = part.Costume3DID
		headOptionalID = nil
	}
	implicitEmptyHead := false
	if query.AccessoryID <= 0 && query.AccessoryCostume3DID <= 0 {
		if emptyHeadID, ok := r.defaultHeadOptionalForRole(role); ok {
			headID = emptyHeadID
			headOptionalID = nil
			implicitEmptyHead = true
		}
	}
	if bodyID <= 0 || headID <= 0 || hairID <= 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo tuple incomplete")
	}
	if !explicitHead && !implicitEmptyHead {
		if officialHeadID, ok := r.officialHeadForRoleTuple(role, bodyID, hairID); ok {
			headID = officialHeadID
			headOptionalID = nil
		}
	}
	if !r.isOfficialPresetTuple(role, bodyID, headID, hairID, headOptionalID) {
		fallbackMode := "auto"
		if explicitHair && explicitHead {
			fallbackMode = "none"
		} else if explicitHair {
			fallbackMode = "hair"
		} else if explicitHead {
			fallbackMode = "head"
		}
		var err error
		headID, hairID, err = r.applyHeadHairFallback(role, fallbackMode, headID, hairID, "3d combo")
		if err != nil {
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
	region = normalizePreview3DRegion(region)
	imageID := fmt.Sprintf("tmp_pjsk3d_%s_%s_combo_c%d_%s_b%d_h%d_r%d_o%d",
		region, cacheSignature, role.CharacterID, unit, bodyID, headID, hairID, optionalID)
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

func (r *preview3DRegistry) anyPartByID(costume3DID int) (preview3DPartEntry, bool) {
	for _, part := range r.parts {
		if part.Costume3DID == costume3DID {
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
	for _, role := range r.characters {
		if !preview3DStatusUsable(role.Status) {
			continue
		}
		if role.Character3DID != query.Character3DID {
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
		if candidates[i].Character3DID != candidates[j].Character3DID {
			return candidates[i].Character3DID < candidates[j].Character3DID
		}
		if candidates[i].CharacterID != candidates[j].CharacterID {
			return candidates[i].CharacterID < candidates[j].CharacterID
		}
		return candidates[i].Unit < candidates[j].Unit
	})
	return candidates
}

func (r *preview3DRegistry) comboAnchorPart(query ComboQuery, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	if query.OutfitID > 0 {
		return r.outfitPartForRole(query.OutfitID, query.OutfitColorID, role)
	}
	if query.BodyCostume3DID > 0 {
		return r.partForRole(query.BodyCostume3DID, role, "body")
	}
	if query.HairCostume3DID > 0 {
		return r.partForRole(query.HairCostume3DID, role, "hair")
	}
	if query.AccessoryID > 0 {
		return r.accessoryPartForRole(query.AccessoryID, query.AccessoryColorID, role)
	}
	if query.AccessoryCostume3DID > 0 {
		if part, ok := r.partForRole(query.AccessoryCostume3DID, role, "head", "head_optional"); ok {
			return part, true
		}
	}
	return r.partForRole(role.BodyCostume3DID, role, "body")
}

func (r *preview3DRegistry) accessoryPartForRole(accessoryID int, colorID int, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if part.CharacterID != role.CharacterID || part.ColorID != colorID {
			continue
		}
		if part.Unit != "" && part.Unit != role.Unit {
			continue
		}
		partAccessoryID := part.AccessoryID
		if partAccessoryID == 0 && part.Costume3DGroupID >= 1000 {
			partAccessoryID = part.Costume3DGroupID / 1000
		}
		if partAccessoryID == accessoryID {
			return part, true
		}
	}
	return preview3DPartEntry{}, false
}

func (r *preview3DRegistry) outfitPartForRole(outfitID int, colorID int, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	for _, part := range r.parts {
		if !preview3DStatusUsable(part.Status) || preview3DPartSlot(part) != "body" {
			continue
		}
		if part.CharacterID != role.CharacterID || part.ColorID != colorID {
			continue
		}
		if part.Unit != "" && part.Unit != role.Unit {
			continue
		}
		partOutfitID := part.OutfitID
		if partOutfitID == 0 && part.Costume3DGroupID >= 1000 {
			partOutfitID = part.Costume3DGroupID / 1000
		}
		if partOutfitID == outfitID {
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

func preview3DPartDiagnostic(part preview3DPartEntry) string {
	details := []string{
		fmt.Sprintf("costume3dId=%d", part.Costume3DID),
		fmt.Sprintf("partType=%s", normalizePreview3DPartType(part.PartType)),
		fmt.Sprintf("unit=%s", part.Unit),
		fmt.Sprintf("status=%s", strings.TrimSpace(part.Status)),
	}
	if part.PackagePath != "" {
		details = append(details, "packagePath="+part.PackagePath)
	}
	if part.BundlePath != "" {
		details = append(details, "bundlePath="+part.BundlePath)
	}
	if part.ColorVariationBundlePath != "" {
		details = append(details, "colorVariationBundlePath="+part.ColorVariationBundlePath)
	}
	if len(part.Warnings) > 0 {
		details = append(details, "warning="+part.Warnings[0])
	}
	return strings.Join(details, " ")
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

func (r *preview3DRegistry) officialHeadForRoleTuple(role preview3DCharacterEntry, bodyID int, hairID int) (int, bool) {
	for _, candidate := range r.characters {
		if !preview3DStatusUsable(candidate.Status) {
			continue
		}
		if candidate.CharacterID != role.CharacterID || candidate.Unit != role.Unit {
			continue
		}
		if candidate.BodyCostume3DID == bodyID && candidate.HairCostume3DID == hairID && candidate.HeadCostume3DID > 0 {
			return candidate.HeadCostume3DID, true
		}
	}
	return 0, false
}

func (r *preview3DRegistry) applyHeadHairFallback(
	role preview3DCharacterEntry,
	fallbackMode string,
	headID int,
	hairID int,
	label string,
) (int, int, error) {
	if !r.headHairBlocked(role.Unit, headID, hairID) {
		return headID, hairID, nil
	}

	if preview3DHeadSideFallbackMode(fallbackMode) || fallbackMode == "auto" {
		if defaultHairID, ok := r.defaultHairForHead(role.Unit, headID); ok {
			if _, usable := r.partForRole(defaultHairID, role, "hair"); usable {
				if !r.headHairBlocked(role.Unit, headID, defaultHairID) {
					return headID, defaultHairID, nil
				}
			}
		}
	}

	if fallbackMode == "hair" || fallbackMode == "auto" {
		if emptyHeadID, ok := r.defaultHeadOptionalForRole(role); ok &&
			!r.headHairBlocked(role.Unit, emptyHeadID, hairID) {
			return emptyHeadID, hairID, nil
		}
	}

	return headID, hairID, fmt.Errorf("%s head/hair combination is blocked: unit=%s head=%d hair=%d", label, role.Unit, headID, hairID)
}

func preview3DHeadSideFallbackMode(mode string) bool {
	return mode == "head" || mode == "head_optional" || mode == "accessory"
}

func (r *preview3DRegistry) headHairBlocked(unit string, headID int, hairID int) bool {
	for _, rule := range r.rules {
		if rule.HeadCostume3DID != headID {
			continue
		}
		if unit != "" && rule.Unit != "" && rule.Unit != unit {
			continue
		}
		if rule.HairCostume3DID == hairID && strings.EqualFold(rule.State, "not_available") {
			return true
		}
	}
	return false
}

func (r *preview3DRegistry) defaultHairForHead(unit string, headID int) (int, bool) {
	var candidates []preview3DCompatibilityRule
	for _, rule := range r.rules {
		if rule.HeadCostume3DID != headID {
			continue
		}
		if unit != "" && rule.Unit != "" && rule.Unit != unit {
			continue
		}
		if !strings.EqualFold(rule.State, "default_hint") {
			continue
		}
		candidates = append(candidates, rule)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Unit == candidates[j].Unit {
			return candidates[i].HairCostume3DID < candidates[j].HairCostume3DID
		}
		if candidates[i].Unit == unit {
			return true
		}
		if candidates[j].Unit == unit {
			return false
		}
		return candidates[i].Unit < candidates[j].Unit
	})
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[0].HairCostume3DID, true
}

func (r *preview3DRegistry) defaultHeadOptionalForRole(role preview3DCharacterEntry) (int, bool) {
	var candidates []preview3DPartEntry
	for _, part := range r.parts {
		if preview3DPartSlot(part) != "head_optional" || !strings.EqualFold(strings.TrimSpace(part.Status), "empty") {
			continue
		}
		if part.CharacterID != role.CharacterID {
			continue
		}
		if part.Unit != "" && part.Unit != role.Unit {
			continue
		}
		candidates = append(candidates, part)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Unit == candidates[j].Unit {
			return candidates[i].Costume3DID < candidates[j].Costume3DID
		}
		if candidates[i].Unit == role.Unit {
			return true
		}
		if candidates[j].Unit == role.Unit {
			return false
		}
		return candidates[i].Unit < candidates[j].Unit
	})
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[0].Costume3DID, true
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
		case "head_and_hair":
			return "head"
		case "head_only", "head_all", "head_front", "head_back":
			return "head_optional"
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
