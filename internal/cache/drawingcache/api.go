package drawingcache

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

var (
	safePathPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	safeExtPattern  = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
)

type API struct {
	dao        *DAO
	storageDir string
	now        func() time.Time
	stats      *cacheStatsTracker
}

func NewAPI(dao *DAO, storageDir string) *API {
	storageDir = strings.TrimSpace(storageDir)
	if storageDir == "" {
		storageDir = "./cache_images"
	}
	return &API{
		dao:        dao,
		storageDir: storageDir,
		now:        time.Now,
		stats:      newCacheStatsTracker(time.Now),
	}
}

func (a *API) RegisterRoutes(router fiber.Router) {
	router.All("/cache/stats", func(c fiber.Ctx) error {
		if c.Method() != fiber.MethodGet {
			return jsonStatus(c, fiber.StatusMethodNotAllowed, fiber.Map{"error": "method not allowed"})
		}
		return a.handleGetCacheStats(c)
	})
	router.All("/cache", func(c fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet:
			return a.handleGetCache(c)
		case fiber.MethodPost:
			return a.handlePostCache(c)
		default:
			return jsonStatus(c, fiber.StatusMethodNotAllowed, fiber.Map{"error": "method not allowed"})
		}
	})
}

func (a *API) handleGetCache(c fiber.Ctx) error {
	apiPathHint := normalizeAPIPath(c.Query("api_path"))
	key := strings.TrimSpace(c.Query("key"))
	if err := ValidateSHA256Key(key); err != nil {
		return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": err.Error()})
	}

	record, err := a.dao.GetRecord(key)
	if err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			a.stats.recordMiss(apiPathHint, cacheMissReasonLookup)
			return jsonStatus(c, fiber.StatusNotFound, fiber.Map{"error": "record not found"})
		}
		return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "query record failed"})
	}

	now := a.now().UTC()
	if !isInfiniteTTL(record.TTLSeconds) && now.After(record.ExpiresAt.UTC()) {
		shared, err := fileReferencedByLiveRecord(a.dao.db, record.FilePath, record.Sha256Key)
		if err != nil {
			return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "query shared file references failed"})
		}
		if !shared {
			if err := removeFileAndPruneEmptyDirsSafe(record.FilePath, a.storageDir); err != nil {
				log.Printf("[drawing-cache-api] lazy delete file failed key=%s path=%s: %v", key, record.FilePath, err)
			}
		}
		if err := a.dao.DeleteRecord(key); err != nil {
			return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "delete expired record failed"})
		}

		a.stats.recordMiss(record.APIPath, cacheMissReasonExpired)
		return jsonStatus(c, fiber.StatusNotFound, fiber.Map{"error": "record expired"})
	}

	exists, err := fileExists(record.FilePath)
	if err != nil {
		return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "check cache file failed"})
	}
	if !exists {
		if err := a.dao.DeleteRecord(key); err != nil {
			return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "delete missing-file record failed"})
		}
		if err := pruneEmptyDirsSafe(filepath.Dir(record.FilePath), a.storageDir); err != nil {
			log.Printf("[drawing-cache-api] prune empty dirs failed key=%s path=%s: %v", key, record.FilePath, err)
		}
		a.stats.recordMiss(record.APIPath, cacheMissReasonMissingFile)
		return jsonStatus(c, fiber.StatusNotFound, fiber.Map{"error": "file not found"})
	}

	if err := a.dao.TouchRecordOnHit(record.Sha256Key, now, record.TTLSeconds); err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			return jsonStatus(c, fiber.StatusNotFound, fiber.Map{"error": "record not found"})
		}
		return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "refresh cache ttl failed"})
	}
	record.LastUsedAt = now
	record.ExpiresAt = cacheExpiresAt(now, record.TTLSeconds)
	a.stats.recordHit(record.APIPath)

	return jsonStatus(c, fiber.StatusOK, fiber.Map{
		"key":          record.Sha256Key,
		"file_path":    record.FilePath,
		"created_at":   record.CreatedAt.UTC().Format(time.RFC3339),
		"last_used_at": record.LastUsedAt.UTC().Format(time.RFC3339),
		"ttl_seconds":  record.TTLSeconds,
		"expires_at":   formatCacheRecordExpiresAt(record),
	})
}

func (a *API) handlePostCache(c fiber.Ctx) error {
	key := strings.TrimSpace(c.FormValue("key"))
	if err := ValidateSHA256Key(key); err != nil {
		return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": err.Error()})
	}

	ttl, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("ttl")), 10, 64)
	if err != nil || ttl < 0 {
		return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": "invalid ttl: must be non-negative integer seconds"})
	}

	apiPath := normalizeAPIPath(c.FormValue("api_path"))
	if apiPath == "" {
		return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": "api_path is required"})
	}

	userID := sanitizePathToken(c.FormValue("user_id"))
	if userID == "" {
		userID = "public"
	}

	filePath := strings.TrimSpace(c.FormValue("file_path"))
	if filePath == "" {
		ext := normalizeFileExt(c.FormValue("ext"))
		apiPathDir := sanitizeStoragePath(apiPath)
		if apiPathDir == "" {
			return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": "api_path is invalid"})
		}
		filePath = filepath.Join(a.storageDir, apiPathDir, userID, fmt.Sprintf("%s.%s", key, ext))
	}

	if !isPathUnderBase(a.storageDir, filePath) {
		return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": "file_path must be inside storage dir"})
	}

	exists, err := fileExists(filePath)
	if err != nil {
		return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "check file failed"})
	}
	if !exists {
		return jsonStatus(c, fiber.StatusBadRequest, fiber.Map{"error": "target file not found"})
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "create storage dir failed"})
	}

	now := a.now().UTC()
	record := &CacheRecord{
		Sha256Key:  key,
		APIPath:    apiPath,
		UserID:     userID,
		FilePath:   filePath,
		CreatedAt:  now,
		LastUsedAt: now,
		TTLSeconds: ttl,
		ExpiresAt:  cacheExpiresAt(now, ttl),
	}

	if err := a.dao.SaveRecord(record); err != nil {
		return jsonStatus(c, fiber.StatusInternalServerError, fiber.Map{"error": "save cache metadata failed"})
	}
	a.stats.recordStore(record.APIPath)

	return jsonStatus(c, fiber.StatusOK, fiber.Map{
		"message":      "ok",
		"key":          key,
		"api_path":     apiPath,
		"user_id":      userID,
		"file_path":    filePath,
		"last_used_at": record.LastUsedAt.UTC().Format(time.RFC3339),
		"ttl_seconds":  record.TTLSeconds,
		"expires_at":   formatCacheRecordExpiresAt(record),
	})
}

func (a *API) handleGetCacheStats(c fiber.Ctx) error {
	filterPath := normalizeAPIPath(c.Query("api_path"))
	return jsonStatus(c, fiber.StatusOK, a.stats.snapshot(filterPath))
}

func normalizeFileExt(ext string) string {
	ext = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(ext)), ".")
	if ext == "" {
		return "png"
	}
	if !safeExtPattern.MatchString(ext) {
		return "png"
	}
	return ext
}

func sanitizePathToken(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return ""
	}
	v = strings.Trim(v, "/")
	v = strings.ReplaceAll(v, "/", "_")
	v = safePathPattern.ReplaceAllString(v, "_")
	v = strings.Trim(v, "._-")
	return v
}

func normalizeAPIPath(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return ""
	}

	v = strings.ReplaceAll(v, "\\", "/")
	parts := strings.Split(v, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = safePathPattern.ReplaceAllString(strings.TrimSpace(part), "-")
		part = strings.Trim(part, "._-")
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/")
}

func sanitizeStoragePath(apiPath string) string {
	normalized := normalizeAPIPath(apiPath)
	if normalized == "" {
		return ""
	}

	parts := strings.Split(normalized, "/")
	for idx, part := range parts {
		parts[idx] = safePathPattern.ReplaceAllString(part, "_")
		parts[idx] = strings.Trim(parts[idx], "._-")
		if parts[idx] == "" {
			return ""
		}
	}
	return filepath.Join(parts...)
}

func isPathUnderBase(base, target string) bool {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func jsonStatus(c fiber.Ctx, status int, payload any) error {
	return c.Status(status).JSON(payload)
}
