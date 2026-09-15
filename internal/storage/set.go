package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/utils/logger"
)

// Slot names one of the five fixed storage slots.
type Slot string

// The five fixed slots.
const (
	SlotAssets     Slot = "assets"
	SlotUserUpload Slot = "user_upload"
	SlotStatic     Slot = "static"
	SlotCache      Slot = "cache"
	SlotImageCache Slot = "image_cache"
)

// Slots lists the fixed slots in configuration order.
var Slots = []Slot{SlotAssets, SlotUserUpload, SlotStatic, SlotCache, SlotImageCache}

// SetConfig is the pjsk_render.storage block: one provider per slot.
type SetConfig struct {
	Assets     ProviderConfig `yaml:"assets"`
	UserUpload ProviderConfig `yaml:"user_upload"`
	Static     ProviderConfig `yaml:"static"`
	Cache      ProviderConfig `yaml:"cache"`
	ImageCache ProviderConfig `yaml:"image_cache"`
}

// Provider returns the block configured for slot.
func (c *SetConfig) Provider(slot Slot) *ProviderConfig {
	switch slot {
	case SlotAssets:
		return &c.Assets
	case SlotUserUpload:
		return &c.UserUpload
	case SlotStatic:
		return &c.Static
	case SlotCache:
		return &c.Cache
	case SlotImageCache:
		return &c.ImageCache
	default:
		return nil
	}
}

// Set holds one Store per slot. BuildSet and Normalized never leave a nil
// field: an unconfigured slot is Disabled().
type Set struct {
	Assets, UserUpload, Static, Cache, ImageCache Store
}

// Store returns the store of slot (nil for an unknown slot).
func (s *Set) Store(slot Slot) *Store {
	switch slot {
	case SlotAssets:
		return &s.Assets
	case SlotUserUpload:
		return &s.UserUpload
	case SlotStatic:
		return &s.Static
	case SlotCache:
		return &s.Cache
	case SlotImageCache:
		return &s.ImageCache
	default:
		return nil
	}
}

// Normalized returns s with every nil slot replaced by Disabled().
func (s Set) Normalized() Set {
	for _, slot := range Slots {
		if store := s.Store(slot); *store == nil {
			*store = Disabled()
		}
	}
	return s
}

// LegacyRoots are the pre-storage directory settings each slot falls back to
// when its provider block is absent.
type LegacyRoots struct {
	AssetPrimary  string // pjsk_render.asset_dirs.primary
	CacheDir      string // pjsk_render.drawing_cache.storage_dir
	ImageCacheDir string // pjsk_render.image_cache.dir
}

func (l LegacyRoots) root(slot Slot) (string, string) {
	switch slot {
	case SlotAssets, SlotUserUpload, SlotStatic:
		return strings.TrimSpace(l.AssetPrimary), "asset_dirs.primary"
	case SlotCache:
		return strings.TrimSpace(l.CacheDir), "drawing_cache.storage_dir"
	default:
		return strings.TrimSpace(l.ImageCacheDir), "image_cache.dir"
	}
}

// Opener builds a store for a resolved provider of one scheme.
type Opener func(Resolved) (Store, error)

// Backends supplies the backends that live outside this package (the s3
// client imports storage, so it is injected by the composition root).
type Backends struct {
	S3 Opener
}

// ErrBackendUnavailable is returned when a slot selects a scheme whose
// backend was not supplied.
var ErrBackendUnavailable = errors.New("storage: backend not available")

// Open validates and opens one slot's provider block. A zero block yields
// Disabled(). Warnings are logged on log (which may be nil).
func Open(slot Slot, c ProviderConfig, backends Backends, log *logger.Logger) (Store, error) {
	if c.IsZero() {
		return Disabled(), nil
	}
	if err := Validate(string(slot), c); err != nil {
		return nil, err
	}
	primary, resolved, err := openProvider(slot, "", c, backends, log)
	if err != nil {
		return nil, err
	}
	logSlotSummary(log, slot, resolved)
	if c.Mirror == nil {
		return primary, nil
	}
	mirror, mirrorResolved, err := openProvider(slot, "mirror.", *c.Mirror, backends, log)
	if err != nil {
		return nil, err
	}
	logSlotSummary(log, Slot(string(slot)+".mirror"), mirrorResolved)
	mode := DualMode(strings.TrimSpace(c.MirrorMode))
	if mode == "" {
		mode = DualWrite
	}
	return NewDual(primary, mirror, mode, log), nil
}

func openProvider(slot Slot, keyPrefix string, c ProviderConfig, backends Backends, log *logger.Logger) (Store, Resolved, error) {
	resolved, err := Resolve(c)
	if err != nil {
		return nil, Resolved{}, fmt.Errorf("storage.%s.%s%w", slot, keyPrefix, err)
	}
	for _, warning := range resolved.Warnings {
		log.Warn("storage provider configuration warning", "slot", string(slot), "detail", keyPrefix+warning)
	}
	if resolved.Scheme == SchemeFS {
		store, openErr := NewLocal(resolved.Root, 0)
		if openErr != nil {
			return nil, Resolved{}, fmt.Errorf("storage.%s.%sroot: %w", slot, keyPrefix, openErr)
		}
		return store, resolved, nil
	}
	if (slot == SlotAssets || slot == SlotImageCache) && keyPrefix == "" && resolved.Root != "" {
		log.Warn("storage slot root must stay empty on s3: the object key already carries the full path",
			"slot", string(slot), "root", resolved.Root)
	}
	if backends.S3 == nil {
		return nil, Resolved{}, fmt.Errorf("storage.%s.%sscheme: %w: %s", slot, keyPrefix, ErrBackendUnavailable, SchemeS3)
	}
	store, openErr := backends.S3(resolved)
	if openErr != nil {
		return nil, Resolved{}, fmt.Errorf("storage.%s.%soptions: %w", slot, keyPrefix, openErr)
	}
	return store, resolved, nil
}

// BuildSet opens every slot. An absent slot block derives a local store from
// its legacy root (or Disabled() when that is empty too); when both a legacy
// root and a slot block are set and disagree, one Warn is logged and the slot
// wins. One Info line per slot summarises the result without credentials.
func BuildSet(cfg SetConfig, legacy LegacyRoots, backends Backends, log *logger.Logger) (Set, error) {
	var set Set
	for _, slot := range Slots {
		store, err := buildSlot(slot, *cfg.Provider(slot), legacy, backends, log)
		if err != nil {
			return Set{}, err
		}
		*set.Store(slot) = store
	}
	return set, nil
}

func buildSlot(slot Slot, c ProviderConfig, legacy LegacyRoots, backends Backends, log *logger.Logger) (Store, error) {
	legacyRoot, legacyKey := legacy.root(slot)
	if !c.IsZero() {
		if legacyRoot != "" {
			if slotRoot, agrees := slotAgreesWithLegacy(c, legacyRoot); !agrees {
				log.Warn("storage slot root disagrees with legacy path",
					"slot", string(slot), "slot_root", slotRoot, "legacy_path", legacyRoot, "legacy_key", legacyKey)
			}
		}
		return Open(slot, c, backends, log)
	}
	if legacyRoot == "" {
		log.Info("storage slot configured", "slot", string(slot), "scheme", "disabled")
		return Disabled(), nil
	}
	// A relative legacy root resolves against the working directory, exactly
	// as the os-based consumers resolve it today.
	absolute, err := filepath.Abs(legacyRoot)
	var store Store
	if err == nil {
		store, err = NewLocal(absolute, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("storage.%s: legacy %s %q: %w", slot, legacyKey, legacyRoot, err)
	}
	logSlotSummary(log, slot, Resolved{Scheme: SchemeFS, Root: absolute, PathStyle: true})
	return store, nil
}

func slotAgreesWithLegacy(c ProviderConfig, legacyRoot string) (string, bool) {
	resolved, err := Resolve(c)
	if err != nil {
		return strings.TrimSpace(c.Root), false
	}
	if resolved.Scheme == SchemeS3 {
		return "s3://" + resolved.Bucket + "/" + resolved.Root, false
	}
	legacyAbs, legacyErr := filepath.Abs(legacyRoot)
	return resolved.Root, legacyErr == nil && resolved.Root != "" && filepath.Clean(resolved.Root) == legacyAbs
}

func logSlotSummary(log *logger.Logger, slot Slot, resolved Resolved) {
	creds := "anonymous"
	if resolved.HasCredentials() {
		creds = "set"
	}
	log.Info("storage slot configured",
		"slot", string(slot),
		"scheme", resolved.Scheme,
		"bucket", resolved.Bucket,
		"root", resolved.Root,
		"endpoints", len(resolved.Endpoints),
		"path_style", resolved.PathStyle,
		"creds", creds,
	)
}
