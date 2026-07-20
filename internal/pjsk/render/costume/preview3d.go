package costume

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"github.com/andybalholm/brotli"
	"github.com/shamaton/msgpack/v3"
	"golang.org/x/sync/singleflight"
)

const (
	defaultPreview3DStaticRelativeDir = "static_images/pjsk_3d_preview"
	compactRegistrySchemaVersion      = 1
)

const (
	preview3DRegistryMaxCompressedBytes = 32 << 20
	preview3DRegistryMaxDecodedBytes    = 128 << 20
	preview3DCaptureMaxResponseBytes    = 64 << 20
	preview3DCaptureAckMaxBytes         = 64 << 10
	preview3DErrorResponseMaxBytes      = 4 << 10
	preview3DCaptureCacheMaxEntries     = 8_192
	preview3DCaptureCacheSweepEvery     = 30 * time.Second
)

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

	registryFlight singleflight.Group
	captureFlight  singleflight.Group
	staticFlight   singleflight.Group
	captureSem     chan struct{}

	mu               sync.Mutex
	cached           map[string]*preview3DRegistry
	cachedAt         map[string]time.Time
	captures         map[string]time.Time
	captureNextSweep time.Time
}

type preview3DRegistryFlightToken byte

type preview3DRegistryFlightResult struct {
	registry   *preview3DRegistry
	err        error
	operations []commandtrace.Stats
	leader     *preview3DRegistryFlightToken
}

type preview3DFlightToken byte

type preview3DFlightResult struct {
	err        error
	operations []commandtrace.Stats
	leader     *preview3DFlightToken
}

type preview3DStaticFlightToken byte

type preview3DStaticFlightResult struct {
	err        error
	operations []commandtrace.Stats
	leader     *preview3DStaticFlightToken
}

type preview3DEndpoint struct {
	region  string
	baseURL string
}

type preview3DRegistry struct {
	characters          []preview3DCharacterEntry
	parts               []preview3DPartEntry
	rules               []preview3DCompatibilityRule
	partRegistryVersion int
}

type preview3DCharacterIndex struct {
	Entries []preview3DCharacterEntry `json:"entries" msgpack:"entries"`
}

type preview3DCharacterEntry struct {
	Character3DID   int    `json:"character3dId" msgpack:"character3dId"`
	CharacterID     int    `json:"characterId" msgpack:"characterId"`
	Unit            string `json:"unit" msgpack:"unit"`
	BodyCostume3DID int    `json:"bodyCostume3dId" msgpack:"bodyCostume3dId"`
	HeadCostume3DID int    `json:"headCostume3dId" msgpack:"headCostume3dId"`
	HairCostume3DID int    `json:"hairCostume3dId" msgpack:"hairCostume3dId"`
	Status          string `json:"status" msgpack:"status"`
}

type preview3DPartRegistry struct {
	Version int                  `json:"version"`
	Entries []preview3DPartEntry `json:"entries"`
}

type preview3DCompactPartRegistry struct {
	SchemaVersion   int
	RegistryVersion int
	Entries         []preview3DPartEntry
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
	BaseSourceKey                string   `json:"baseSourceKey"`
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

type preview3DCompactCompatibilityRegistry struct {
	SchemaVersion int
	Rules         []preview3DCompatibilityRule
}

type preview3DCompatibilityRule struct {
	Unit            string `json:"unit"`
	HeadCostume3DID int    `json:"headCostume3dId"`
	HairCostume3DID int    `json:"hairCostume3dId"`
	State           string `json:"state"`
	IsDefault       bool   `json:"isDefault"`
}

type preview3DSelection struct {
	ImageID                 string
	RoleID                  string
	BodyCostume3DID         int
	HeadCostume3DID         int
	HeadPackagePath         string
	HairCostume3DID         int
	HeadOptionalCostume3DID *int
	AccessoryID             int
	CharacterID             int
	Unit                    string
	Costume3DGroupID        int
	ColorID                 int
}

type preview3DAccessoryCatalogEntry struct {
	AccessoryID               int
	RepresentativeCostume3DID int
	Costume3DIDs              []int
	Character3DIDs            []int
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
		captures:   make(map[string]time.Time),
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	selection, err := registry.resolveQuery(region, costume3DID, query, s.captureCacheSignature())
	finishPrepare()
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	selection, err := registry.resolveQuery(region, costume3DID, query, s.captureCacheSignature())
	finishPrepare()
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	selection, err := registry.resolveCombo(region, query, s.captureCacheSignature())
	finishPrepare()
	if err != nil {
		return nil, err
	}
	if err := s.ensureCapture(ctx, endpoint, selection, "temporary"); err != nil {
		return nil, err
	}
	return s.getCapture(ctx, endpoint, selection.ImageID)
}

func (s *Preview3DService) HairIDsForRole(ctx context.Context, region string, character3DID int) (map[int]int, error) {
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	roles := registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
	if len(roles) != 1 {
		return nil, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
	}
	parts := registry.hairPartsForRole(roles[0])
	ids := make(map[int]int, len(parts))
	for index, part := range parts {
		ids[part.Costume3DID] = index + 1
	}
	return ids, nil
}

func (s *Preview3DService) AccessoryIDsForRole(ctx context.Context, region string, character3DID int) (map[int][]int, error) {
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	roles := registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
	if len(roles) != 1 {
		return nil, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
	}
	return registry.accessoryIDsForRole(roles[0]), nil
}

func (s *Preview3DService) AccessoryCostume3DIDForRole(ctx context.Context, region string, accessoryID int, colorID int, character3DID int) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("3d preview service is not configured")
	}
	endpoint, err := s.endpointForRegion(region)
	if err != nil {
		return 0, err
	}
	registry, err := s.registry(ctx, endpoint)
	if err != nil {
		return 0, err
	}
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	roles := registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
	if len(roles) != 1 {
		return 0, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
	}
	part, ok := registry.accessoryPartForRole(accessoryID, colorID, roles[0])
	if !ok {
		return 0, registry.accessoryNotUsableError(accessoryID, colorID, character3DID, roles[0])
	}
	return part.Costume3DID, nil
}

func (s *Preview3DService) OutfitIDsForRole(ctx context.Context, region string, character3DID int) (map[int]int, error) {
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	roles := registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
	if len(roles) != 1 {
		return nil, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
	}
	return registry.outfitIDsForRole(roles[0]), nil
}

func (s *Preview3DService) OutfitCostume3DIDForRole(ctx context.Context, region string, outfitID int, colorID int, character3DID int) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("3d preview service is not configured")
	}
	endpoint, err := s.endpointForRegion(region)
	if err != nil {
		return 0, err
	}
	registry, err := s.registry(ctx, endpoint)
	if err != nil {
		return 0, err
	}
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	roles := registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
	if len(roles) != 1 {
		return 0, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
	}
	part, ok := registry.outfitPartForRole(outfitID, colorID, roles[0])
	if !ok {
		return 0, fmt.Errorf("3d combo outfit not usable: outfit=%d character3d=%d color=%d", outfitID, character3DID, colorID)
	}
	return part.Costume3DID, nil
}

func (s *Preview3DService) HairCostume3DIDForRole(ctx context.Context, region string, hairID int, character3DID int) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("3d preview service is not configured")
	}
	endpoint, err := s.endpointForRegion(region)
	if err != nil {
		return 0, err
	}
	registry, err := s.registry(ctx, endpoint)
	if err != nil {
		return 0, err
	}
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	roles := registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
	if len(roles) != 1 {
		return 0, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
	}
	part, ok := registry.hairPartForRole(hairID, roles[0])
	if !ok {
		return 0, fmt.Errorf("3d combo hair not usable: hair=%d character3d=%d", hairID, character3DID)
	}
	return part.Costume3DID, nil
}

func (s *Preview3DService) AccessoryCatalog(ctx context.Context, region string, character3DID int) ([]preview3DAccessoryCatalogEntry, error) {
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
	finishPrepare := commandtrace.MeasureOperation(ctx, "preview3d.prepare")
	defer finishPrepare()
	var roles []preview3DCharacterEntry
	if character3DID > 0 {
		roles = registry.comboRoleCandidates(ComboQuery{Character3DID: character3DID})
		if len(roles) != 1 {
			return nil, fmt.Errorf("3d combo role not found: character3d=%d", character3DID)
		}
	} else {
		for id := 1; id <= 31; id++ {
			candidates := registry.comboRoleCandidates(ComboQuery{Character3DID: id})
			if len(candidates) == 1 {
				roles = append(roles, candidates[0])
			}
		}
	}
	return registry.accessoryCatalog(roles), nil
}

func (s *Preview3DService) registry(ctx context.Context, endpoint preview3DEndpoint) (*preview3DRegistry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	key := endpoint.key()
	finishLookup := commandtrace.MeasureOperation(ctx, "preview3d.registry_lookup")
	s.mu.Lock()
	cached := s.validCachedRegistryLocked(endpoint, time.Now())
	s.mu.Unlock()
	finishLookup()
	if cached != nil {
		return cached, nil
	}

	callerToken := new(preview3DRegistryFlightToken)
	resultCh := s.registryFlight.DoChan(key, func() (any, error) {
		timeout := 3 * s.cfg.Timeout
		if timeout <= 0 {
			timeout = 45 * time.Second
		}
		detached := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
		sharedBase, cancel := context.WithTimeout(detached, timeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		complete := func(result preview3DRegistryFlightResult) preview3DRegistryFlightResult {
			result.operations = trace.Snapshot().Operations
			return result
		}

		finishLookup := commandtrace.MeasureOperation(sharedCtx, "preview3d.registry_lookup")
		s.mu.Lock()
		cached := s.validCachedRegistryLocked(endpoint, time.Now())
		s.mu.Unlock()
		finishLookup()
		if cached != nil {
			return complete(preview3DRegistryFlightResult{registry: cached, leader: callerToken}), nil
		}

		registry, err := s.fetchRegistry(sharedCtx, endpoint)

		if err == nil {
			s.mu.Lock()
			s.cached[key] = registry
			s.cachedAt[key] = time.Now()
			s.mu.Unlock()
		}
		return complete(preview3DRegistryFlightResult{registry: registry, err: err, leader: callerToken}), nil
	})

	finishWait := commandtrace.MeasureOperation(ctx, "preview3d.registry_wait")
	select {
	case <-ctx.Done():
		finishWait()
		return nil, ctx.Err()
	case completed := <-resultCh:
		finishWait()
		if completed.Err != nil {
			return nil, completed.Err
		}
		result, ok := completed.Val.(preview3DRegistryFlightResult)
		if !ok {
			return nil, fmt.Errorf("3d preview registry returned unexpected shared result %T", completed.Val)
		}
		commandtrace.MergeOperations(ctx, result.operations)
		if result.leader != callerToken {
			commandtrace.RecordOperation(ctx, "preview3d.registry_shared", 0)
		}
		return result.registry, result.err
	}
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
	if err := s.getMessagePackRegistry(ctx, endpoint, "/runtime/character3d-index.msgpack.br", &characterIndex, false); err != nil {
		return nil, err
	}
	partRegistry, err := s.getPartRegistry(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	compatibility, err := s.getCompatibilityRegistry(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	registry := &preview3DRegistry{
		characters:          characterIndex.Entries,
		parts:               partRegistry.Entries,
		rules:               compatibility.Rules,
		partRegistryVersion: partRegistry.Version,
	}
	if err := registry.validateAccessoryIdentity(); err != nil {
		return nil, err
	}
	return registry, nil
}

func (s *Preview3DService) getPartRegistry(ctx context.Context, endpoint preview3DEndpoint) (preview3DPartRegistry, error) {
	const requestPath = "/runtime/parts/part-registry-compact.msgpack.br"
	var compact preview3DCompactPartRegistry
	if err := s.getMessagePackRegistry(ctx, endpoint, requestPath, &compact, true); err != nil {
		return preview3DPartRegistry{}, err
	}
	if compact.SchemaVersion != compactRegistrySchemaVersion {
		return preview3DPartRegistry{}, fmt.Errorf("3d preview registry %s has unsupported compact schema %d", requestPath, compact.SchemaVersion)
	}
	return preview3DPartRegistry{Version: compact.RegistryVersion, Entries: compact.Entries}, nil
}

func (s *Preview3DService) getCompatibilityRegistry(ctx context.Context, endpoint preview3DEndpoint) (preview3DCompatibilityRegistry, error) {
	const requestPath = "/runtime/parts/head-hair-compatibility-compact.msgpack.br"
	var compact preview3DCompactCompatibilityRegistry
	if err := s.getMessagePackRegistry(ctx, endpoint, requestPath, &compact, true); err != nil {
		return preview3DCompatibilityRegistry{}, err
	}
	if compact.SchemaVersion != compactRegistrySchemaVersion {
		return preview3DCompatibilityRegistry{}, fmt.Errorf("3d preview registry %s has unsupported compact schema %d", requestPath, compact.SchemaVersion)
	}
	return preview3DCompatibilityRegistry{Rules: compact.Rules}, nil
}

func (s *Preview3DService) getRegistryResponse(
	ctx context.Context,
	endpoint preview3DEndpoint,
	requestPath string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(endpoint, requestPath), nil)
	if err != nil {
		return nil, err
	}
	finishHTTP := commandtrace.MeasureOperation(ctx, "preview3d.registry_http")
	resp, err := s.client.Do(req)
	finishHTTP()
	if err != nil {
		return nil, fmt.Errorf("3d preview registry request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("3d preview registry %s returned HTTP %d", requestPath, resp.StatusCode)
	}
	return resp, nil
}

func (s *Preview3DService) getMessagePackRegistry(
	ctx context.Context,
	endpoint preview3DEndpoint,
	requestPath string,
	out any,
	asArray bool,
) error {
	resp, err := s.getRegistryResponse(ctx, endpoint, requestPath)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	finishDecode := commandtrace.MeasureOperation(ctx, "preview3d.registry_decode")
	if resp.ContentLength > preview3DRegistryMaxCompressedBytes {
		finishDecode()
		return fmt.Errorf("3d preview registry response exceeds %d bytes", preview3DRegistryMaxCompressedBytes)
	}
	resp.Body = io.NopCloser(brotli.NewReader(resp.Body))
	packed, err := readPreview3DResponse(resp, preview3DRegistryMaxDecodedBytes, "3d preview registry")
	if err == nil {
		var decodeErr error
		if asArray {
			decodeErr = msgpack.UnmarshalAsArray(packed, out)
		} else {
			decodeErr = msgpack.Unmarshal(packed, out)
		}
		if decodeErr != nil {
			err = fmt.Errorf("3d preview registry %s decode failed: %w", requestPath, decodeErr)
		}
	}
	finishDecode()
	return err
}

func (s *Preview3DService) captureExists(ctx context.Context, endpoint preview3DEndpoint, imageID string) bool {
	finishLookup := commandtrace.MeasureOperation(ctx, "preview3d.capture_lookup")
	if s.cachedCaptureExists(endpoint, imageID) {
		finishLookup()
		return true
	}
	finishLookup()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.url(endpoint, "/captures/"+imageID+".png"), nil)
	if err != nil {
		return false
	}
	finishHTTP := commandtrace.MeasureOperation(ctx, "preview3d.capture_head")
	resp, err := s.client.Do(req)
	finishHTTP()
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
	now := time.Now()
	s.sweepCaptureCacheLocked(now)
	expiresAt, ok := s.captures[key]
	if !ok {
		return false
	}
	if now.Before(expiresAt) {
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
	now := time.Now()
	s.sweepCaptureCacheLocked(now)
	s.captures[endpoint.captureKey(imageID)] = now.Add(s.cfg.CaptureExistsTTL)
}

func (s *Preview3DService) sweepCaptureCacheLocked(now time.Time) {
	if s.captures == nil {
		s.captures = make(map[string]time.Time)
	}
	if now.Before(s.captureNextSweep) && len(s.captures) < preview3DCaptureCacheMaxEntries {
		return
	}
	for key, expiresAt := range s.captures {
		if !now.Before(expiresAt) {
			delete(s.captures, key)
		}
	}
	s.captureNextSweep = now.Add(preview3DCaptureCacheSweepEvery)

	if len(s.captures) < preview3DCaptureCacheMaxEntries {
		return
	}
	// The existence cache is only an optimization. Evict a batch instead of
	// paying an O(n) oldest-entry search on every insertion at capacity.
	target := preview3DCaptureCacheMaxEntries * 3 / 4
	for key := range s.captures {
		delete(s.captures, key)
		if len(s.captures) <= target {
			break
		}
	}
}

func (s *Preview3DService) ensureCapture(ctx context.Context, endpoint preview3DEndpoint, selection preview3DSelection, cacheMode string) error {
	if s.captureExists(ctx, endpoint, selection.ImageID) {
		return nil
	}
	callerToken := new(preview3DFlightToken)
	resultCh := s.captureFlight.DoChan(endpoint.captureKey(selection.ImageID), func() (any, error) {
		timeout := s.cfg.CaptureAcquireTimeout + 3*s.cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		detached := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
		sharedBase, cancel := context.WithTimeout(detached, timeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		result := preview3DFlightResult{leader: callerToken}
		if s.captureExists(sharedCtx, endpoint, selection.ImageID) {
			result.operations = trace.Snapshot().Operations
			return result, nil
		}
		release, err := s.acquireCapturePermit(sharedCtx)
		if err != nil {
			result.err = err
			result.operations = trace.Snapshot().Operations
			return result, nil
		}
		defer release()
		if s.captureExists(sharedCtx, endpoint, selection.ImageID) {
			result.operations = trace.Snapshot().Operations
			return result, nil
		}
		if err := s.captureSelection(sharedCtx, endpoint, selection, cacheMode); err != nil {
			result.err = err
			result.operations = trace.Snapshot().Operations
			return result, nil
		}
		s.markCaptureExists(endpoint, selection.ImageID)
		result.operations = trace.Snapshot().Operations
		return result, nil
	})
	finishWait := commandtrace.MeasureOperation(ctx, "preview3d.capture_wait")
	select {
	case <-ctx.Done():
		finishWait()
		return ctx.Err()
	case completed := <-resultCh:
		finishWait()
		if completed.Err != nil {
			return completed.Err
		}
		result, ok := completed.Val.(preview3DFlightResult)
		if !ok {
			return fmt.Errorf("3d preview capture returned unexpected shared result %T", completed.Val)
		}
		commandtrace.MergeOperations(ctx, result.operations)
		if result.leader != callerToken {
			commandtrace.RecordOperation(ctx, "preview3d.capture_shared", 0)
		}
		return result.err
	}
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
	finishQueue := commandtrace.MeasureOperation(ctx, "preview3d.capture_queue")
	select {
	case s.captureSem <- struct{}{}:
		finishQueue()
		cancel()
		return func() { <-s.captureSem }, nil
	case <-waitCtx.Done():
		finishQueue()
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
	if selection.HeadPackagePath != "" {
		body["headPackagePath"] = selection.HeadPackagePath
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
	finishEncode := commandtrace.MeasureOperation(ctx, "preview3d.capture_encode")
	payload, err := json.Marshal(body)
	finishEncode()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url(endpoint, "/capture"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	finishHTTP := commandtrace.MeasureOperation(ctx, "preview3d.capture_http")
	resp, err := s.client.Do(req)
	if err != nil {
		finishHTTP()
		return fmt.Errorf("3d preview capture request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, preview3DErrorResponseMaxBytes))
		finishHTTP()
		return fmt.Errorf("3d preview capture returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	_, err = readPreview3DResponse(resp, preview3DCaptureAckMaxBytes, "3d preview capture")
	finishHTTP()
	if err != nil {
		return fmt.Errorf("3d preview capture response failed: %w", err)
	}
	return nil
}

func (s *Preview3DService) getCapture(ctx context.Context, endpoint preview3DEndpoint, imageID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(endpoint, "/captures/"+imageID+".png"), nil)
	if err != nil {
		return nil, err
	}
	finishHTTP := commandtrace.MeasureOperation(ctx, "preview3d.fetch_http")
	resp, err := s.client.Do(req)
	if err != nil {
		finishHTTP()
		return nil, fmt.Errorf("3d preview capture fetch request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, preview3DErrorResponseMaxBytes))
		finishHTTP()
		return nil, fmt.Errorf("3d preview capture fetch returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	data, err := readPreview3DResponse(resp, preview3DCaptureMaxResponseBytes, "3d preview capture fetch")
	finishHTTP()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func readPreview3DResponse(resp *http.Response, maxBytes int64, label string) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("%s response body is unavailable", label)
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("%s response exceeds %d bytes", label, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s response read failed: %w", label, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s response exceeds %d bytes", label, maxBytes)
	}
	return data, nil
}

func (s *Preview3DService) ensureStaticCaptureFile(ctx context.Context, endpoint preview3DEndpoint, imageID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	staticOutputDir := strings.TrimSpace(s.cfg.StaticOutputDir)
	if staticOutputDir == "" || strings.TrimSpace(imageID) == "" {
		return nil
	}
	target := filepath.Join(staticOutputDir, imageID+".png")
	finishLookup := commandtrace.MeasureOperation(ctx, "preview3d.static_lookup")
	if _, err := os.Stat(target); err == nil {
		finishLookup()
		return nil
	} else if !os.IsNotExist(err) {
		finishLookup()
		return err
	}
	finishLookup()

	callerToken := new(preview3DStaticFlightToken)
	resultCh := s.staticFlight.DoChan(target, func() (any, error) {
		sharedBase, cancel := s.staticCaptureSharedContext()
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		complete := func(err error) preview3DStaticFlightResult {
			return preview3DStaticFlightResult{
				err:        err,
				operations: trace.Snapshot().Operations,
				leader:     callerToken,
			}
		}

		// Another process or an earlier waiter may have published the target
		// between the caller-side fast path and this shared worker.
		finishStat := commandtrace.MeasureOperation(sharedCtx, "preview3d.static_stat")
		if _, err := os.Stat(target); err == nil {
			finishStat()
			return complete(nil), nil
		} else if !os.IsNotExist(err) {
			finishStat()
			return complete(err), nil
		}
		finishStat()

		// readPreview3DResponse allocates this slice for the shared worker; it
		// never aliases caller-owned memory or escapes to another waiter.
		data, err := s.getCapture(sharedCtx, endpoint, imageID)
		if err != nil {
			return complete(err), nil
		}
		finishMkdir := commandtrace.MeasureOperation(sharedCtx, "preview3d.static_mkdir")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			finishMkdir()
			return complete(err), nil
		}
		finishMkdir()

		finishStore := commandtrace.MeasureOperation(sharedCtx, "preview3d.store")
		err = writePreview3DCaptureAtomically(sharedCtx, target, data)
		finishStore()
		return complete(err), nil
	})

	finishWait := commandtrace.MeasureOperation(ctx, "preview3d.static_wait")
	select {
	case <-ctx.Done():
		finishWait()
		return ctx.Err()
	case completed := <-resultCh:
		finishWait()
		if completed.Err != nil {
			return completed.Err
		}
		result, ok := completed.Val.(preview3DStaticFlightResult)
		if !ok {
			return fmt.Errorf("3d preview static capture returned unexpected shared result %T", completed.Val)
		}
		commandtrace.MergeOperations(ctx, result.operations)
		if result.leader != callerToken {
			commandtrace.RecordOperation(ctx, "preview3d.static_shared", 0)
		}
		return result.err
	}
}

func (s *Preview3DService) staticCaptureSharedContext() (context.Context, context.CancelFunc) {
	timeout := s.cfg.Timeout + 30*time.Second
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	shared := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
	return context.WithTimeout(shared, timeout)
}

func writePreview3DCaptureAtomically(ctx context.Context, targetPath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	finishWrite := commandtrace.MeasureOperation(ctx, "preview3d.static_write")
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		finishWrite()
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		finishWrite()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		finishWrite()
		return err
	}
	if err := tmp.Close(); err != nil {
		finishWrite()
		return err
	}
	finishWrite()
	if err := ctx.Err(); err != nil {
		return err
	}
	finishRename := commandtrace.MeasureOperation(ctx, "preview3d.static_rename")
	err = os.Rename(tmpName, targetPath)
	finishRename()
	return err
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
	accessoryIDs := r.accessoryIDsForRaw(costume3DID)
	if len(accessoryIDs) > 1 {
		return preview3DSelection{}, fmt.Errorf("3d preview accessory raw id is ambiguous: raw=%d ids=%v", costume3DID, accessoryIDs)
	}
	accessoryID := 0
	if len(accessoryIDs) == 1 {
		accessoryID = accessoryIDs[0]
	}
	var selected preview3DPartEntry
	var ok bool
	if accessoryID > 0 {
		selected, ok = r.accessoryPartByRawID(costume3DID, accessoryID)
	} else {
		selected, ok = r.partByID(costume3DID)
	}
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
	headPackagePath := ""
	if candidate, ok, err := r.strictHeadPartForRole(headID, role); err != nil {
		return preview3DSelection{}, err
	} else if ok {
		headPackagePath = candidate.PackagePath
	}
	hairID := role.HairCostume3DID
	var headOptionalID *int
	for _, partType := range []string{"body", "hair"} {
		candidate, ok := r.groupPart(selected, role.Unit, partType)
		if !ok {
			continue
		}
		switch partType {
		case "body":
			bodyID = candidate.Costume3DID
		case "hair":
			hairID = candidate.Costume3DID
		}
	}
	selectedSlot := preview3DPartSlot(selected)
	if selectedSlot != "head" && selectedSlot != "head_optional" {
		if officialHeadID, official := r.officialHeadForRoleTuple(role, bodyID, hairID); official {
			candidate, ok, err := r.strictHeadPartForRole(officialHeadID, role)
			if err != nil {
				return preview3DSelection{}, err
			}
			if ok {
				headID = candidate.Costume3DID
				headPackagePath = candidate.PackagePath
				headOptionalID = nil
			}
		} else if candidate, ok, err := r.strictGroupHeadPart(selected, role); err != nil {
			return preview3DSelection{}, err
		} else if ok {
			headID = candidate.Costume3DID
			headPackagePath = candidate.PackagePath
			headOptionalID = nil
		}
	}
	switch selectedSlot {
	case "body":
		bodyID = selected.Costume3DID
	case "head":
		headID = selected.Costume3DID
		headPackagePath = selected.PackagePath
	case "hair":
		hairID = selected.Costume3DID
	case "head_optional":
		headID = selected.Costume3DID
		headPackagePath = selected.PackagePath
		headOptionalID = nil
	}
	if bodyID <= 0 || headID <= 0 || hairID <= 0 {
		return preview3DSelection{}, fmt.Errorf("3d preview tuple incomplete for costume %d", costume3DID)
	}
	if slot := preview3DPartSlot(selected); slot != "head" && slot != "head_optional" {
		if officialHeadID, ok := r.officialHeadForRoleTuple(role, bodyID, hairID); ok {
			headID = officialHeadID
			headPackagePath = ""
			if candidate, found, err := r.strictHeadPartForRole(officialHeadID, role); err != nil {
				return preview3DSelection{}, err
			} else if found {
				headPackagePath = candidate.PackagePath
			}
			if preview3DPartSlot(selected) != "head_optional" {
				headOptionalID = nil
			}
		}
	}
	if !r.isOfficialPresetTuple(role, bodyID, headID, hairID, headOptionalID) {
		previousHeadID := headID
		var err error
		headID, hairID, err = r.applyHeadHairFallback(role, preview3DPartSlot(selected), headID, hairID, "3d preview")
		if err != nil {
			return preview3DSelection{}, err
		}
		if headID != previousHeadID {
			headPackagePath = ""
			if candidate, found := r.defaultHeadOptionalPartForRole(role); found && candidate.Costume3DID == headID {
				headPackagePath = candidate.PackagePath
			}
		}
	}
	if accessoryID > 0 && strings.TrimSpace(headPackagePath) == "" {
		return preview3DSelection{}, fmt.Errorf("3d preview accessory source has no packagePath: accessory=%d raw=%d", accessoryID, headID)
	}

	optionalID := 0
	if headOptionalID != nil {
		optionalID = *headOptionalID
	}
	unit := sanitizePreview3DImagePart(role.Unit)
	region = normalizePreview3DRegion(region)
	imageID := fmt.Sprintf("pjsk3d_%s_c%d_%s_i%d_b%d_h%d_r%d_o%d",
		region, role.CharacterID, unit, costume3DID, bodyID, headID, hairID, optionalID)
	imageID = appendPreview3DHeadIdentity(imageID, accessoryID, headPackagePath)
	return preview3DSelection{
		ImageID:                 imageID,
		RoleID:                  fmt.Sprintf("%d:%s", role.CharacterID, role.Unit),
		BodyCostume3DID:         bodyID,
		HeadCostume3DID:         headID,
		HeadPackagePath:         headPackagePath,
		HairCostume3DID:         hairID,
		HeadOptionalCostume3DID: headOptionalID,
		AccessoryID:             accessoryID,
		CharacterID:             role.CharacterID,
		Unit:                    role.Unit,
		Costume3DGroupID:        selected.Costume3DGroupID,
		ColorID:                 selected.ColorID,
	}, nil
}

func (r *preview3DRegistry) resolveQuery(region string, costume3DID int, query Query, cacheSignature string) (preview3DSelection, error) {
	if query.Character3DID > 0 && (query.OutfitID > 0 || query.AccessoryID > 0 || query.HairID > 0) {
		return r.resolveCombo(region, ComboQuery{
			Region:           region,
			OutfitID:         query.OutfitID,
			OutfitColorID:    query.ColorID,
			AccessoryID:      query.AccessoryID,
			AccessoryColorID: query.ColorID,
			HairID:           query.HairID,
			Character3DID:    query.Character3DID,
		}, cacheSignature)
	}
	if query.Character3DID > 0 {
		combo := ComboQuery{Region: region, Character3DID: query.Character3DID}
		switch partType, _ := normalizePartType(query.ExpectedPartType); partType {
		case "body":
			combo.BodyCostume3DID = costume3DID
		case "head":
			combo.AccessoryCostume3DID = costume3DID
		case "hair":
			combo.HairCostume3DID = costume3DID
		}
		if combo.BodyCostume3DID > 0 || combo.AccessoryCostume3DID > 0 || combo.HairCostume3DID > 0 {
			return r.resolveCombo(region, combo, cacheSignature)
		}
	}
	return r.resolve(region, costume3DID)
}

func (r *preview3DRegistry) resolveCombo(region string, query ComboQuery, cacheSignature string) (preview3DSelection, error) {
	if query.AccessoryID > 0 && query.AccessoryCostume3DID > 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo accessory id and raw id cannot be used together")
	}
	roles := r.comboRoleCandidates(query)
	if len(roles) == 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo role not found: character3d=%d", query.Character3DID)
	}
	if len(roles) > 1 {
		return preview3DSelection{}, fmt.Errorf("3d combo character3d id is duplicated: %d", query.Character3DID)
	}
	role := roles[0]
	resolvedAccessoryID := query.AccessoryID
	if query.AccessoryCostume3DID > 0 {
		accessoryIDs := r.accessoryIDsForRole(role)[query.AccessoryCostume3DID]
		if len(accessoryIDs) > 1 {
			return preview3DSelection{}, fmt.Errorf("3d combo accessory raw id is ambiguous: raw=%d ids=%v", query.AccessoryCostume3DID, accessoryIDs)
		}
		if len(accessoryIDs) == 1 {
			resolvedAccessoryID = accessoryIDs[0]
		}
	}
	anchor, ok, err := r.comboAnchorPart(query, role)
	if err != nil {
		return preview3DSelection{}, err
	}
	if !ok {
		if query.OutfitID > 0 {
			return preview3DSelection{}, fmt.Errorf("3d combo outfit not usable: outfit=%d character3d=%d color=%d", query.OutfitID, query.Character3DID, query.OutfitColorID)
		}
		if query.AccessoryID > 0 {
			return preview3DSelection{}, r.accessoryNotUsableError(query.AccessoryID, query.AccessoryColorID, query.Character3DID, role)
		}
		return preview3DSelection{}, fmt.Errorf("3d combo anchor part not found")
	}
	explicitBody := query.OutfitID > 0 || query.BodyCostume3DID > 0
	explicitHair := query.HairID > 0 || query.HairCostume3DID > 0
	explicitHead := query.AccessoryID > 0 || query.AccessoryCostume3DID > 0
	useAnchorGroup := (explicitBody || explicitHair || explicitHead) && anchor.Costume3DGroupID > 0

	bodyID := role.BodyCostume3DID
	headID := role.HeadCostume3DID
	headPackagePath := ""
	if candidate, ok, err := r.strictHeadPartForRole(headID, role); err != nil {
		return preview3DSelection{}, err
	} else if ok {
		headPackagePath = candidate.PackagePath
	}
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
	if !explicitBody && useAnchorGroup {
		if part, ok := r.groupPart(anchor, role.Unit, "body"); ok {
			bodyID = part.Costume3DID
		} else if anchor.Costume3DGroupID >= 1000 {
			if part, ok := r.outfitPartForRole(anchor.Costume3DGroupID/1000, anchor.ColorID, role); ok {
				bodyID = part.Costume3DID
			}
		}
	}
	if !explicitHair && useAnchorGroup {
		if part, ok := r.groupPart(anchor, role.Unit, "hair"); ok {
			hairID = part.Costume3DID
		}
	}
	if query.HairID > 0 {
		part, ok := r.hairPartForRole(query.HairID, role)
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo hair not usable: hair=%d character3d=%d", query.HairID, query.Character3DID)
		}
		hairID = part.Costume3DID
	} else if query.HairCostume3DID > 0 {
		part, ok := r.partForRole(query.HairCostume3DID, role, "hair")
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo hair part not usable for unit=%s: %d", role.Unit, query.HairCostume3DID)
		}
		hairID = part.Costume3DID
	}
	if query.AccessoryID > 0 {
		part, ok := r.accessoryPartForRole(query.AccessoryID, query.AccessoryColorID, role)
		if !ok {
			return preview3DSelection{}, r.accessoryNotUsableError(query.AccessoryID, query.AccessoryColorID, query.Character3DID, role)
		}
		headID = part.Costume3DID
		headPackagePath = part.PackagePath
		if strings.TrimSpace(headPackagePath) == "" {
			return preview3DSelection{}, fmt.Errorf("3d combo accessory source has no packagePath: accessory=%d raw=%d", query.AccessoryID, part.Costume3DID)
		}
		headOptionalID = nil
	} else if query.AccessoryCostume3DID > 0 {
		var part preview3DPartEntry
		var ok bool
		if resolvedAccessoryID > 0 {
			part, ok = r.accessoryPartByRawIDForRole(query.AccessoryCostume3DID, resolvedAccessoryID, role)
		} else {
			var resolveErr error
			part, ok, resolveErr = r.strictHeadPartForRole(query.AccessoryCostume3DID, role)
			if resolveErr != nil {
				return preview3DSelection{}, resolveErr
			}
		}
		if !ok {
			return preview3DSelection{}, fmt.Errorf("3d combo head/accessory part not usable for unit=%s: %d", role.Unit, query.AccessoryCostume3DID)
		}
		headID = part.Costume3DID
		headPackagePath = part.PackagePath
		if resolvedAccessoryID > 0 && strings.TrimSpace(headPackagePath) == "" {
			return preview3DSelection{}, fmt.Errorf("3d combo accessory source has no packagePath: accessory=%d raw=%d", resolvedAccessoryID, part.Costume3DID)
		}
		headOptionalID = nil
	}
	implicitHead := false
	if !explicitHead {
		groupHeadAnchor := anchor
		if explicitHair {
			if part, ok := r.partForRole(hairID, role, "hair"); ok {
				groupHeadAnchor = part
			}
		}
		if groupHeadAnchor.Costume3DGroupID > 0 {
			groupHead, ok, err := r.strictGroupHeadPart(groupHeadAnchor, role)
			if err != nil {
				return preview3DSelection{}, err
			}
			if ok {
				headID = groupHead.Costume3DID
				headPackagePath = groupHead.PackagePath
				headOptionalID = nil
				implicitHead = true
			}
		}
		if !implicitHead && explicitHair {
			defaultHead, ok, err := r.defaultHeadForHair(role, hairID)
			if err != nil {
				return preview3DSelection{}, err
			}
			if ok {
				headID = defaultHead.Costume3DID
				headPackagePath = defaultHead.PackagePath
				headOptionalID = nil
				implicitHead = true
			}
		}
		if !implicitHead {
			emptyHead, ok := r.defaultHeadOptionalPartForRole(role)
			if ok {
				headID = emptyHead.Costume3DID
				headPackagePath = emptyHead.PackagePath
				headOptionalID = nil
				implicitHead = true
			}
		}
	}
	if bodyID <= 0 || headID <= 0 || hairID <= 0 {
		return preview3DSelection{}, fmt.Errorf("3d combo tuple incomplete")
	}
	if !explicitHead && !implicitHead {
		if officialHeadID, ok := r.officialHeadForRoleTuple(role, bodyID, hairID); ok {
			headID = officialHeadID
			headPackagePath = ""
			if candidate, found, err := r.strictHeadPartForRole(officialHeadID, role); err != nil {
				return preview3DSelection{}, err
			} else if found {
				headPackagePath = candidate.PackagePath
			}
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
		previousHeadID := headID
		headID, hairID, err = r.applyHeadHairFallback(role, fallbackMode, headID, hairID, "3d combo")
		if err != nil {
			return preview3DSelection{}, err
		}
		if headID != previousHeadID {
			headPackagePath = ""
			if candidate, found := r.defaultHeadOptionalPartForRole(role); found && candidate.Costume3DID == headID {
				headPackagePath = candidate.PackagePath
			}
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
	imageID = appendPreview3DHeadIdentity(imageID, resolvedAccessoryID, headPackagePath)
	return preview3DSelection{
		ImageID:                 imageID,
		RoleID:                  fmt.Sprintf("%d:%s", role.CharacterID, role.Unit),
		BodyCostume3DID:         bodyID,
		HeadCostume3DID:         headID,
		HeadPackagePath:         headPackagePath,
		HairCostume3DID:         hairID,
		HeadOptionalCostume3DID: headOptionalID,
		AccessoryID:             resolvedAccessoryID,
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

func (r *preview3DRegistry) comboAnchorPart(query ComboQuery, role preview3DCharacterEntry) (preview3DPartEntry, bool, error) {
	if query.OutfitID > 0 {
		part, ok := r.outfitPartForRole(query.OutfitID, query.OutfitColorID, role)
		return part, ok, nil
	}
	if query.BodyCostume3DID > 0 {
		part, ok := r.partForRole(query.BodyCostume3DID, role, "body")
		return part, ok, nil
	}
	if query.HairID > 0 {
		part, ok := r.hairPartForRole(query.HairID, role)
		return part, ok, nil
	}
	if query.HairCostume3DID > 0 {
		part, ok := r.partForRole(query.HairCostume3DID, role, "hair")
		return part, ok, nil
	}
	if query.AccessoryID > 0 {
		part, ok := r.accessoryPartForRole(query.AccessoryID, query.AccessoryColorID, role)
		return part, ok, nil
	}
	if query.AccessoryCostume3DID > 0 {
		return r.strictHeadPartForRole(query.AccessoryCostume3DID, role)
	}
	part, ok := r.partForRole(role.BodyCostume3DID, role, "body")
	return part, ok, nil
}

func (r *preview3DRegistry) hairPartForRole(hairID int, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	parts := r.hairPartsForRole(role)
	if hairID < 1 || hairID > len(parts) {
		return preview3DPartEntry{}, false
	}
	return parts[hairID-1], true
}

func (r *preview3DRegistry) hairPartsForRole(role preview3DCharacterEntry) []preview3DPartEntry {
	byID := make(map[int]preview3DPartEntry)
	for _, part := range r.parts {
		if !preview3DStatusUsable(part.Status) || preview3DPartSlot(part) != "hair" {
			continue
		}
		if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
			continue
		}
		byID[part.Costume3DID] = part
	}
	parts := make([]preview3DPartEntry, 0, len(byID))
	for _, part := range byID {
		parts = append(parts, part)
	}
	sort.Slice(parts, func(i, j int) bool {
		leftDefault := parts[i].Costume3DID == role.HairCostume3DID
		rightDefault := parts[j].Costume3DID == role.HairCostume3DID
		if leftDefault != rightDefault {
			return leftDefault
		}
		return parts[i].Costume3DID < parts[j].Costume3DID
	})
	return parts
}

func (r *preview3DRegistry) accessoryPartForRole(accessoryID int, colorID int, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	accessoryIDs := r.accessoryIDsByIdentity()
	var candidates []preview3DPartEntry
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
		if r.accessoryID(part, accessoryIDs) == accessoryID {
			candidates = append(candidates, part)
		}
	}
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftExact := candidates[i].Unit == role.Unit
		rightExact := candidates[j].Unit == role.Unit
		if leftExact != rightExact {
			return leftExact
		}
		return candidates[i].Costume3DID < candidates[j].Costume3DID
	})
	return candidates[0], true
}

func (r *preview3DRegistry) legacyAccessoryIDsForRole(legacyID int, role preview3DCharacterEntry) []int {
	if legacyID <= 0 {
		return nil
	}
	byIdentity := r.accessoryIDsByIdentity()
	ids := make(map[int]struct{})
	exact := false
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
			continue
		}
		accessoryID := r.accessoryID(part, byIdentity)
		if accessoryID <= 0 {
			continue
		}
		if accessoryID == legacyID {
			exact = true
		}
		if part.Costume3DGroupID >= 1000 && part.Costume3DGroupID/1000 == legacyID {
			ids[accessoryID] = struct{}{}
		}
	}
	if exact {
		return nil
	}
	result := make([]int, 0, len(ids))
	for accessoryID := range ids {
		result = append(result, accessoryID)
	}
	sort.Ints(result)
	return result
}

func (r *preview3DRegistry) accessoryNotUsableError(accessoryID int, colorID int, character3DID int, role preview3DCharacterEntry) error {
	if candidates := r.legacyAccessoryIDsForRole(accessoryID, role); len(candidates) > 0 {
		return &LegacyAccessoryIDError{LegacyID: accessoryID, Character3DID: character3DID, AccessoryIDs: candidates}
	}
	return fmt.Errorf("3d combo accessory not usable: accessory=%d character3d=%d color=%d", accessoryID, character3DID, colorID)
}

func (r *preview3DRegistry) accessoryIDsForRole(role preview3DCharacterEntry) map[int][]int {
	byIdentity := r.accessoryIDsByIdentity()
	idSets := make(map[int]map[int]struct{})
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
			continue
		}
		accessoryID := r.accessoryID(part, byIdentity)
		if accessoryID <= 0 {
			continue
		}
		if idSets[part.Costume3DID] == nil {
			idSets[part.Costume3DID] = make(map[int]struct{})
		}
		idSets[part.Costume3DID][accessoryID] = struct{}{}
	}
	ids := make(map[int][]int, len(idSets))
	for rawID, set := range idSets {
		values := make([]int, 0, len(set))
		for accessoryID := range set {
			values = append(values, accessoryID)
		}
		sort.Ints(values)
		ids[rawID] = values
	}
	return ids
}

func (r *preview3DRegistry) accessoryIDsForRaw(costume3DID int) []int {
	byIdentity := r.accessoryIDsByIdentity()
	set := make(map[int]struct{})
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if part.Costume3DID != costume3DID || !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if accessoryID := r.accessoryID(part, byIdentity); accessoryID > 0 {
			set[accessoryID] = struct{}{}
		}
	}
	ids := make([]int, 0, len(set))
	for accessoryID := range set {
		ids = append(ids, accessoryID)
	}
	sort.Ints(ids)
	return ids
}

func (r *preview3DRegistry) accessoryPartByRawID(costume3DID int, accessoryID int) (preview3DPartEntry, bool) {
	byIdentity := r.accessoryIDsByIdentity()
	var candidates []preview3DPartEntry
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if part.Costume3DID != costume3DID || !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if r.accessoryID(part, byIdentity) == accessoryID {
			candidates = append(candidates, part)
		}
	}
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CharacterID != candidates[j].CharacterID {
			return candidates[i].CharacterID < candidates[j].CharacterID
		}
		if candidates[i].Unit != candidates[j].Unit {
			return candidates[i].Unit < candidates[j].Unit
		}
		return candidates[i].PackagePath < candidates[j].PackagePath
	})
	return candidates[0], true
}

func (r *preview3DRegistry) accessoryPartByRawIDForRole(costume3DID int, accessoryID int, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	byIdentity := r.accessoryIDsByIdentity()
	var candidates []preview3DPartEntry
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if part.Costume3DID != costume3DID || !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
			continue
		}
		if r.accessoryID(part, byIdentity) == accessoryID {
			candidates = append(candidates, part)
		}
	}
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftExact := candidates[i].Unit == role.Unit
		rightExact := candidates[j].Unit == role.Unit
		if leftExact != rightExact {
			return leftExact
		}
		return candidates[i].PackagePath < candidates[j].PackagePath
	})
	return candidates[0], true
}

func (r *preview3DRegistry) accessoryCatalog(roles []preview3DCharacterEntry) []preview3DAccessoryCatalogEntry {
	type roleAccessory struct {
		rawIDs         map[int]struct{}
		representative int
		rank           int
	}
	type catalogBuilder struct {
		rawIDs         map[int]struct{}
		roleIDs        map[int]struct{}
		representative int
	}
	byIdentity := r.accessoryIDsByIdentity()
	builders := make(map[int]*catalogBuilder)
	for _, role := range roles {
		byAccessoryID := make(map[int]*roleAccessory)
		for _, part := range r.parts {
			slot := preview3DPartSlot(part)
			if !preview3DStatusUsable(part.Status) || part.ColorID != 1 || (slot != "head" && slot != "head_optional") {
				continue
			}
			if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
				continue
			}
			accessoryID := r.accessoryID(part, byIdentity)
			if accessoryID <= 0 {
				continue
			}
			group := byAccessoryID[accessoryID]
			if group == nil {
				group = &roleAccessory{rawIDs: make(map[int]struct{}), rank: 2}
				byAccessoryID[accessoryID] = group
			}
			group.rawIDs[part.Costume3DID] = struct{}{}
			rank := 1
			if part.Unit == role.Unit {
				rank = 0
			}
			if rank < group.rank || (rank == group.rank && (group.representative == 0 || part.Costume3DID < group.representative)) {
				group.rank = rank
				group.representative = part.Costume3DID
			}
		}
		for accessoryID, roleGroup := range byAccessoryID {
			builder := builders[accessoryID]
			if builder == nil {
				builder = &catalogBuilder{rawIDs: make(map[int]struct{}), roleIDs: make(map[int]struct{})}
				builders[accessoryID] = builder
			}
			for rawID := range roleGroup.rawIDs {
				builder.rawIDs[rawID] = struct{}{}
			}
			builder.roleIDs[role.Character3DID] = struct{}{}
			if builder.representative == 0 || roleGroup.representative < builder.representative {
				builder.representative = roleGroup.representative
			}
		}
	}
	entries := make([]preview3DAccessoryCatalogEntry, 0, len(builders))
	for accessoryID, builder := range builders {
		rawIDs := make([]int, 0, len(builder.rawIDs))
		for rawID := range builder.rawIDs {
			rawIDs = append(rawIDs, rawID)
		}
		sort.Ints(rawIDs)
		roleIDs := make([]int, 0, len(builder.roleIDs))
		for roleID := range builder.roleIDs {
			roleIDs = append(roleIDs, roleID)
		}
		sort.Ints(roleIDs)
		entries = append(entries, preview3DAccessoryCatalogEntry{
			AccessoryID:               accessoryID,
			RepresentativeCostume3DID: builder.representative,
			Costume3DIDs:              rawIDs,
			Character3DIDs:            roleIDs,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].AccessoryID < entries[j].AccessoryID })
	return entries
}

func (r *preview3DRegistry) accessoryIDsByIdentity() map[string]int {
	if r != nil && r.partRegistryVersion >= 2 {
		return nil
	}
	originalIDBySource := make(map[string]int)
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		source := strings.TrimSpace(part.BaseSourceKey)
		if (slot != "head" && slot != "head_optional") || part.ColorID != 1 || source == "" || part.Costume3DGroupID < 1000 {
			continue
		}
		setMinimumPositive(originalIDBySource, source, part.Costume3DGroupID)
	}
	sourceByFamily, err := r.accessorySourceByFamily()
	if err != nil {
		return map[string]int{}
	}
	sourceByGroupSlot := r.accessorySourceByGroupSlot()
	sourceForID := make(map[int]string)
	invalidIDs := make(map[int]struct{})
	for source, id := range originalIDBySource {
		if previous := sourceForID[id]; previous != "" && previous != source {
			invalidIDs[id] = struct{}{}
			continue
		}
		sourceForID[id] = source
	}
	ids := make(map[string]int, len(sourceByFamily)+len(sourceByGroupSlot)+len(originalIDBySource))
	for source, id := range originalIDBySource {
		if _, invalid := invalidIDs[id]; id > 0 && !invalid {
			ids[preview3DAccessorySourceIdentity(source)] = id
		}
	}
	for family, source := range sourceByFamily {
		id := originalIDBySource[source]
		if _, invalid := invalidIDs[id]; id > 0 && !invalid {
			ids[family] = id
		}
	}
	for groupSlot, source := range sourceByGroupSlot {
		id := originalIDBySource[source]
		if _, invalid := invalidIDs[id]; id > 0 && !invalid {
			ids[groupSlot] = id
		}
	}
	return ids
}

func (r *preview3DRegistry) accessorySourceByFamily() (map[string]string, error) {
	sourceByFamily := make(map[string]string)
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		source := strings.TrimSpace(part.BaseSourceKey)
		if (slot != "head" && slot != "head_optional") || part.ColorID != 1 || source == "" || part.Costume3DGroupID < 1000 {
			continue
		}
		family := preview3DAccessoryFamily(part)
		if previous := sourceByFamily[family]; previous != "" && previous != source {
			return nil, fmt.Errorf("accessory family %s resolves to multiple base sources", family)
		}
		sourceByFamily[family] = source
	}
	return sourceByFamily, nil
}

func (r *preview3DRegistry) accessorySourceByGroupSlot() map[string]string {
	sources := make(map[string]map[string]struct{})
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		source := strings.TrimSpace(part.BaseSourceKey)
		if (slot != "head" && slot != "head_optional") || part.ColorID != 1 || source == "" || part.Costume3DGroupID < 1000 {
			continue
		}
		key := preview3DAccessoryGroupSlot(part)
		if sources[key] == nil {
			sources[key] = make(map[string]struct{})
		}
		sources[key][source] = struct{}{}
	}
	unique := make(map[string]string)
	for key, candidates := range sources {
		if len(candidates) != 1 {
			continue
		}
		for source := range candidates {
			unique[key] = source
		}
	}
	return unique
}

func (r *preview3DRegistry) validateAccessoryIdentity() error {
	if r == nil || r.partRegistryVersion < 2 {
		return nil
	}
	sourceByFamily, err := r.accessorySourceByFamily()
	if err != nil {
		return fmt.Errorf("3d preview registry accessory identity is invalid: %w", err)
	}
	sourceByGroupSlot := r.accessorySourceByGroupSlot()
	originalIDBySource := make(map[string]int)
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		source := strings.TrimSpace(part.BaseSourceKey)
		if (slot != "head" && slot != "head_optional") || part.ColorID != 1 || part.Costume3DGroupID < 1000 || source == "" {
			continue
		}
		setMinimumPositive(originalIDBySource, source, part.Costume3DGroupID)
	}
	sourceByID := make(map[int]string)
	for source, id := range originalIDBySource {
		if previous := sourceByID[id]; previous != "" && previous != source {
			return fmt.Errorf("3d preview registry accessory identity is invalid: accessoryId %d maps to sources %s and %s", id, previous, source)
		}
		sourceByID[id] = source
	}
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if slot != "head" && slot != "head_optional" {
			if part.AccessoryID > 0 {
				return fmt.Errorf("3d preview registry accessory identity is invalid: accessoryId %d is set on non-accessory part %d", part.AccessoryID, part.Costume3DID)
			}
			continue
		}
		if part.Costume3DGroupID < 1000 {
			if part.AccessoryID > 0 {
				return fmt.Errorf("3d preview registry accessory identity is invalid: accessoryId %d has no original-color source", part.AccessoryID)
			}
			continue
		}
		candidateSources := make(map[string]struct{}, 2)
		partSource := strings.TrimSpace(part.BaseSourceKey)
		if originalIDBySource[partSource] > 0 {
			candidateSources[partSource] = struct{}{}
		}
		if familySource := sourceByFamily[preview3DAccessoryFamily(part)]; familySource != "" {
			candidateSources[familySource] = struct{}{}
		}
		if groupSource := sourceByGroupSlot[preview3DAccessoryGroupSlot(part)]; groupSource != "" {
			candidateSources[groupSource] = struct{}{}
		}
		if part.AccessoryID <= 0 {
			if partSource != "" {
				return fmt.Errorf("3d preview registry accessory identity is invalid: source %s has no accessoryId", partSource)
			}
			continue
		}
		if partSource == "" {
			return fmt.Errorf("3d preview registry accessory identity is invalid: accessoryId %d has no base source", part.AccessoryID)
		}
		if len(candidateSources) == 0 {
			return fmt.Errorf("3d preview registry accessory identity is invalid: accessoryId %d has no original-color source", part.AccessoryID)
		}
		if len(candidateSources) > 1 {
			return fmt.Errorf("3d preview registry accessory identity is invalid: accessory %d resolves to multiple original-color sources", part.Costume3DID)
		}
		var resolvedSource string
		for source := range candidateSources {
			resolvedSource = source
		}
		expectedID := originalIDBySource[resolvedSource]
		if part.AccessoryID != expectedID {
			return fmt.Errorf("3d preview registry accessory identity is invalid: source %s expects accessoryId %d, got %d", resolvedSource, expectedID, part.AccessoryID)
		}
	}
	return nil
}

func setMinimumPositive(values map[string]int, key string, candidate int) {
	if candidate <= 0 {
		return
	}
	if current := values[key]; current == 0 || candidate < current {
		values[key] = candidate
	}
}

func preview3DAccessoryFamily(part preview3DPartEntry) string {
	return fmt.Sprintf("%d|%s|%s", part.Costume3DGroupID, strings.TrimSpace(part.Unit), preview3DPartSlot(part))
}

func preview3DAccessoryGroupSlot(part preview3DPartEntry) string {
	return fmt.Sprintf("group|%d|%s", part.Costume3DGroupID, preview3DPartSlot(part))
}

func preview3DAccessorySourceIdentity(source string) string {
	return "source|" + strings.TrimSpace(source)
}

func appendPreview3DHeadIdentity(imageID string, accessoryID int, packagePath string) string {
	if accessoryID > 0 {
		imageID += fmt.Sprintf("_a%d", accessoryID)
	}
	packagePath = strings.TrimSpace(packagePath)
	if packagePath != "" {
		sum := sha256.Sum256([]byte(packagePath))
		imageID += "_s" + hex.EncodeToString(sum[:])[:10]
	}
	return imageID
}

func (r *preview3DRegistry) accessoryID(part preview3DPartEntry, byIdentity map[string]int) int {
	if r != nil && r.partRegistryVersion >= 2 {
		return part.AccessoryID
	}
	directID := byIdentity[preview3DAccessorySourceIdentity(part.BaseSourceKey)]
	familyID := byIdentity[preview3DAccessoryFamily(part)]
	groupSlotID := byIdentity[preview3DAccessoryGroupSlot(part)]
	ids := make(map[int]struct{}, 3)
	for _, id := range []int{directID, familyID, groupSlotID} {
		if id > 0 {
			ids[id] = struct{}{}
		}
	}
	if len(ids) != 1 {
		return 0
	}
	for id := range ids {
		return id
	}
	return 0
}

func (r *preview3DRegistry) outfitPartForRole(outfitID int, colorID int, role preview3DCharacterEntry) (preview3DPartEntry, bool) {
	var candidates []preview3DPartEntry
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
			candidates = append(candidates, part)
		}
	}
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftExact := candidates[i].Unit == role.Unit
		rightExact := candidates[j].Unit == role.Unit
		if leftExact != rightExact {
			return leftExact
		}
		return candidates[i].Costume3DID < candidates[j].Costume3DID
	})
	return candidates[0], true
}

func (r *preview3DRegistry) outfitIDsForRole(role preview3DCharacterEntry) map[int]int {
	ids := make(map[int]int)
	for _, part := range r.parts {
		if !preview3DStatusUsable(part.Status) || preview3DPartSlot(part) != "body" {
			continue
		}
		if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
			continue
		}
		outfitID := part.OutfitID
		if outfitID == 0 && part.Costume3DGroupID >= 1000 {
			outfitID = part.Costume3DGroupID / 1000
		}
		if outfitID > 0 {
			ids[part.Costume3DID] = outfitID
		}
	}
	return ids
}

func (r *preview3DRegistry) partForRole(costume3DID int, role preview3DCharacterEntry, partTypes ...string) (preview3DPartEntry, bool) {
	allowed := make(map[string]struct{}, len(partTypes))
	for _, partType := range partTypes {
		allowed[normalizePreview3DPartType(partType)] = struct{}{}
	}
	var candidates []preview3DPartEntry
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
		candidates = append(candidates, part)
	}
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftExact := candidates[i].Unit == role.Unit
		rightExact := candidates[j].Unit == role.Unit
		if leftExact != rightExact {
			return leftExact
		}
		return preview3DPartSlot(candidates[i]) < preview3DPartSlot(candidates[j])
	})
	return candidates[0], true
}

func (r *preview3DRegistry) strictHeadPartForRole(costume3DID int, role preview3DCharacterEntry) (preview3DPartEntry, bool, error) {
	var candidates []preview3DPartEntry
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if part.Costume3DID != costume3DID || !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if part.CharacterID != role.CharacterID || (part.Unit != "" && part.Unit != role.Unit) {
			continue
		}
		candidates = append(candidates, part)
	}
	return strictPreview3DHeadPart(candidates, role, fmt.Sprintf("3d head raw id is ambiguous: raw=%d", costume3DID))
}

func (r *preview3DRegistry) strictGroupHeadPart(selected preview3DPartEntry, role preview3DCharacterEntry) (preview3DPartEntry, bool, error) {
	var matching []preview3DPartEntry
	bestRank := make(map[string]int, 2)
	for _, part := range r.parts {
		slot := preview3DPartSlot(part)
		if !preview3DStatusUsable(part.Status) || (slot != "head" && slot != "head_optional") {
			continue
		}
		if part.CharacterID != selected.CharacterID || part.Costume3DGroupID != selected.Costume3DGroupID {
			continue
		}
		if role.Unit != "" && part.Unit != "" && part.Unit != role.Unit {
			continue
		}
		matching = append(matching, part)
		rank := preview3DColorRank(part.ColorID, selected.ColorID)
		if current, found := bestRank[slot]; !found || rank < current {
			bestRank[slot] = rank
		}
	}
	candidates := matching[:0]
	for _, part := range matching {
		if preview3DColorRank(part.ColorID, selected.ColorID) == bestRank[preview3DPartSlot(part)] {
			candidates = append(candidates, part)
		}
	}
	ambiguity := fmt.Sprintf("3d group head source is ambiguous: group=%d color=%d", selected.Costume3DGroupID, selected.ColorID)
	return strictPreview3DHeadPart(candidates, role, ambiguity)
}

func strictPreview3DHeadPart(candidates []preview3DPartEntry, role preview3DCharacterEntry, ambiguity string) (preview3DPartEntry, bool, error) {
	bySource := make(map[string]preview3DPartEntry)
	for _, part := range candidates {
		key := preview3DHeadSourceKey(part)
		current, found := bySource[key]
		if !found || preferPreview3DHeadPart(part, current, role) {
			bySource[key] = part
		}
	}
	if len(bySource) == 0 {
		return preview3DPartEntry{}, false, nil
	}
	keys := make([]string, 0, len(bySource))
	for key := range bySource {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 1 {
		return preview3DPartEntry{}, false, fmt.Errorf(
			"%s character3d=%d character=%d unit=%s sources=%v",
			ambiguity, role.Character3DID, role.CharacterID, role.Unit, keys,
		)
	}
	return bySource[keys[0]], true, nil
}

func preview3DHeadSourceKey(part preview3DPartEntry) string {
	prefix := "slot|" + preview3DPartSlot(part) + "|"
	if packagePath := strings.TrimSpace(part.PackagePath); packagePath != "" {
		return prefix + "package|" + packagePath
	}
	if source := strings.TrimSpace(part.BaseSourceKey); source != "" {
		return prefix + "source|" + source
	}
	if part.AccessoryID > 0 {
		return fmt.Sprintf("%saccessory|%d", prefix, part.AccessoryID)
	}
	return fmt.Sprintf("%sgroup|%d|unit|%s", prefix, part.Costume3DGroupID, strings.TrimSpace(part.Unit))
}

func preferPreview3DHeadPart(candidate preview3DPartEntry, current preview3DPartEntry, role preview3DCharacterEntry) bool {
	candidateExact := candidate.Unit == role.Unit
	currentExact := current.Unit == role.Unit
	if candidateExact != currentExact {
		return candidateExact
	}
	return candidate.ColorID == 1 && current.ColorID != 1
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
		if emptyHead, ok := r.defaultHeadOptionalPartForRole(role); ok &&
			!r.headHairBlocked(role.Unit, emptyHead.Costume3DID, hairID) {
			return emptyHead.Costume3DID, hairID, nil
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
		if !preview3DCompatibilityRuleIsDefault(rule) {
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

func (r *preview3DRegistry) defaultHeadForHair(role preview3DCharacterEntry, hairID int) (preview3DPartEntry, bool, error) {
	var candidates []preview3DPartEntry
	for _, rule := range r.rules {
		if rule.HairCostume3DID != hairID || !preview3DCompatibilityRuleIsDefault(rule) {
			continue
		}
		if role.Unit != "" && rule.Unit != "" && rule.Unit != role.Unit {
			continue
		}
		part, ok, err := r.strictHeadPartForRole(rule.HeadCostume3DID, role)
		if err != nil {
			return preview3DPartEntry{}, false, err
		}
		if ok {
			candidates = append(candidates, part)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftExact := candidates[i].Unit == role.Unit
		rightExact := candidates[j].Unit == role.Unit
		if leftExact != rightExact {
			return leftExact
		}
		leftOriginal := candidates[i].ColorID == 1
		rightOriginal := candidates[j].ColorID == 1
		if leftOriginal != rightOriginal {
			return leftOriginal
		}
		return candidates[i].Costume3DID < candidates[j].Costume3DID
	})
	if len(candidates) == 0 {
		return preview3DPartEntry{}, false, nil
	}
	return candidates[0], true, nil
}

func preview3DCompatibilityRuleIsDefault(rule preview3DCompatibilityRule) bool {
	return !strings.EqualFold(rule.State, "not_available") &&
		(rule.IsDefault || strings.EqualFold(rule.State, "default_hint"))
}

func (r *preview3DRegistry) defaultHeadOptionalPartForRole(role preview3DCharacterEntry) (preview3DPartEntry, bool) {
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
		return preview3DPartEntry{}, false
	}
	return candidates[0], true
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
