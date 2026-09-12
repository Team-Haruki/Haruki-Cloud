package deck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	json "haruki-cloud/internal/jsonutil"
)

const (
	registryPollTimeout      = 15 * time.Second
	registryManifestMaxBytes = 8 << 20
)

type deckMasterdataSignature struct {
	Dir   string
	Hash  string
	Files int
}

func (p *remoteEngineProvider) startMasterdataRefreshLoop() {
	if p == nil || p.masterdataRefreshInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(p.masterdataRefreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			p.refreshMasterdataSignatures()
		}
	}()
}

func (p *remoteEngineProvider) refreshMasterdataSignatures() {
	if p == nil {
		return
	}
	p.mu.Lock()
	recommenders := make([]*RemoteDeckRecommender, 0, len(p.recommenders))
	for _, recommender := range p.recommenders {
		if remote, ok := recommender.(*RemoteDeckRecommender); ok && remote != nil {
			recommenders = append(recommenders, remote)
		}
	}
	p.mu.Unlock()

	for _, recommender := range recommenders {
		recommender.refreshMasterdataSignature()
	}
}

func (r *RemoteDeckRecommender) captureMasterdataSignature() {
	if r == nil {
		return
	}
	if r.registryURL != "" {
		r.pollRegistryVersion()
		return
	}
	signature, err := deckMasterdataDirSignature(r.masterdataDir, r.region)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("deck masterdata signature failed",
				"operation", "capture_initial",
				"region", r.region,
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		return
	}
	r.masterdataMu.Lock()
	r.masterdataSig = signature.Hash
	r.masterdataMu.Unlock()
}

func (r *RemoteDeckRecommender) refreshMasterdataSignature() {
	if r == nil {
		return
	}
	if r.registryURL != "" {
		r.pollRegistryVersion()
		return
	}
	signature, err := deckMasterdataDirSignature(r.masterdataDir, r.region)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("deck masterdata signature failed",
				"operation", "refresh",
				"region", r.region,
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		return
	}

	r.masterdataMu.Lock()
	previous := r.masterdataSig
	if previous == "" {
		r.masterdataSig = signature.Hash
		r.masterdataMu.Unlock()
		return
	}
	if previous == signature.Hash {
		r.masterdataMu.Unlock()
		return
	}
	r.masterdataSig = signature.Hash
	r.masterdataMu.Unlock()

	for _, state := range r.targetStates {
		r.invalidate(state, remoteRewarmMasterdata)
	}
	if r.logger != nil {
		r.logger.Info("deck masterdata changed",
			"region", r.region,
			"files", signature.Files,
			"action", "refresh_on_next_request",
		)
	}
}

func registryCurrentURL(baseURL, region string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/master/" + strings.ToLower(strings.TrimSpace(region)) + "/current"
}

type registryManifestPointer struct {
	ContentHash string `json:"contentHash"`
}

func (r *RemoteDeckRecommender) currentRegistryContentHash() string {
	if r == nil {
		return ""
	}
	r.masterdataMu.Lock()
	defer r.masterdataMu.Unlock()
	return r.registryContentHash
}

// adoptRegistryContentHash records the hash deck-service reports when Cloud
// has not polled the registry yet. A later poll that sees a different hash
// invalidates as usual.
func (r *RemoteDeckRecommender) adoptRegistryContentHash(hash string) {
	hash = strings.TrimSpace(hash)
	if r == nil || hash == "" {
		return
	}
	r.masterdataMu.Lock()
	defer r.masterdataMu.Unlock()
	if r.registryContentHash == "" {
		r.registryContentHash = hash
	}
}

// pollRegistryVersion is the registry-mode counterpart of the directory
// signature: one conditional GET of the region's manifest pointer. The first
// observation is only recorded; every later change invalidates the targets
// so their next request re-runs the readiness handshake.
func (r *RemoteDeckRecommender) pollRegistryVersion() {
	if r == nil || r.registryURL == "" {
		return
	}
	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), registryPollTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, registryCurrentURL(r.registryURL, r.region), nil)
	if err != nil {
		r.logRegistryPollFailure("build_request", err)
		return
	}
	r.masterdataMu.Lock()
	etag := r.registryETag
	r.masterdataMu.Unlock()
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := client.Do(request)
	if err != nil {
		r.logRegistryPollFailure("fetch", err)
		return
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotModified:
		return
	case http.StatusOK:
	default:
		r.logRegistryPollFailure("fetch", fmt.Errorf("registry returned HTTP %d", response.StatusCode))
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, registryManifestMaxBytes))
	if err != nil {
		r.logRegistryPollFailure("read", err)
		return
	}
	var pointer registryManifestPointer
	if err := json.Unmarshal(body, &pointer); err != nil {
		r.logRegistryPollFailure("decode", err)
		return
	}
	hash := strings.TrimSpace(pointer.ContentHash)
	if hash == "" {
		r.logRegistryPollFailure("decode", fmt.Errorf("manifest has no contentHash"))
		return
	}

	r.masterdataMu.Lock()
	previous := r.registryContentHash
	r.registryContentHash = hash
	r.registryETag = response.Header.Get("ETag")
	r.masterdataMu.Unlock()
	if previous == "" || previous == hash {
		return
	}

	for _, state := range r.targetStates {
		r.invalidate(state, remoteRewarmMasterdata)
	}
	if r.logger != nil {
		r.logger.Info("deck masterdata changed",
			"region", r.region,
			"source", "registry",
			"action", "refresh_on_next_request",
		)
	}
}

func (r *RemoteDeckRecommender) logRegistryPollFailure(operation string, err error) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Warn("deck masterdata registry poll failed",
		"operation", operation,
		"region", r.region,
		"error_type", fmt.Sprintf("%T", err),
	)
}

func deckMasterdataDirSignature(configured, region string) (deckMasterdataSignature, error) {
	dir, ok := resolveDeckMasterdataContentDir(configured, region)
	if !ok {
		return deckMasterdataSignature{}, fmt.Errorf("masterdata dir not found for region %s under %s", strings.TrimSpace(region), strings.TrimSpace(configured))
	}

	builder := &deckMasterdataSignatureBuilder{dir: dir, hasher: sha256.New()}
	err := filepath.WalkDir(dir, builder.visit)
	if err != nil {
		return deckMasterdataSignature{}, err
	}
	if builder.files == 0 {
		return deckMasterdataSignature{}, fmt.Errorf("masterdata dir %s has no json files", dir)
	}
	return deckMasterdataSignature{
		Dir:   dir,
		Hash:  hex.EncodeToString(builder.hasher.Sum(nil)),
		Files: builder.files,
	}, nil
}

type deckMasterdataSignatureBuilder struct {
	dir    string
	hasher hash.Hash
	files  int
}

func (b *deckMasterdataSignatureBuilder) visit(path string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() {
		if entry.Name() == ".git" {
			return filepath.SkipDir
		}
		return nil
	}
	if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(b.dir, path)
	if err != nil {
		rel = path
	}
	b.files++
	_, err = fmt.Fprintf(b.hasher, "%s\x00%d\x00%d\x00", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
	return err
}
