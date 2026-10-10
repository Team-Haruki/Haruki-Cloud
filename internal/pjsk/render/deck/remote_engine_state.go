package deck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"haruki-cloud/internal/httpcoding"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
)

const (
	remoteStateTimeout   = 5 * time.Second
	maxRemoteStateBytes  = 1 << 20
	remoteStatePath      = "/state/masterdata"
	musicMetaPathHashTag = "path:"
)

// remoteDeckState is what deck-service reports it has loaded. `musicMetas`
// (deck-service ≥ the zstd release) covers every region whichever path
// loaded it; older registry-mode services only report musicMetasDigest per
// registry region.
type remoteDeckState struct {
	Regions map[string]struct {
		ContentHash      string `json:"contentHash"`
		MusicMetasDigest string `json:"musicMetasDigest"`
	} `json:"regions"`
	MusicMetas map[string]string `json:"musicMetas"`
}

func (s *remoteDeckState) musicMetasDigest(region string) string {
	if s == nil {
		return ""
	}
	if digest := strings.TrimSpace(s.MusicMetas[region]); digest != "" {
		return digest
	}
	return strings.TrimSpace(s.Regions[region].MusicMetasDigest)
}

// registryOwnsMusicMetas reports whether the target loaded the region from
// the master registry and holds music metas it fetched for it. Such a target
// keeps the region's metas current by itself, so Cloud never pushes its own
// copy there: Cloud refreshes that copy on its own, slower interval, and a push
// made while it lags the registry rolled the target back to the previous metas.
func (s *remoteDeckState) registryOwnsMusicMetas(region string) bool {
	if s == nil {
		return false
	}
	entry, ok := s.Regions[region]
	return ok && strings.TrimSpace(entry.ContentHash) != "" && strings.TrimSpace(entry.MusicMetasDigest) != ""
}

func (s *remoteDeckState) contentHash(region string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.Regions[region].ContentHash)
}

// fetchRemoteDeckState asks the target what it already holds. Errors are
// returned to the caller, which then falls back to pushing as before.
func (r *RemoteDeckRecommender) fetchRemoteDeckState(ctx context.Context, exec *remoteExecution) (*remoteDeckState, error) {
	baseURL := strings.TrimSpace(exec.BaseURL())
	if r == nil || r.client == nil || baseURL == "" {
		return nil, fmt.Errorf("deck-service target is not configured")
	}
	ctx, cancel := context.WithTimeout(normalizeRecommendContext(ctx), remoteStateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+remoteStatePath, nil)
	if err != nil {
		return nil, err
	}
	httpcoding.SetRequestHeaders(req.Header, "")
	finish := commandtrace.MeasureOperation(ctx, "deck.ready_probe")
	resp, err := r.client.Do(req)
	if err != nil {
		finish()
		return nil, err
	}
	defer resp.Body.Close()
	r.coding.Observe(baseURL, resp.Header)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteStateBytes+1))
	finish()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deck-service %s returned HTTP %d", remoteStatePath, resp.StatusCode)
	}
	if len(raw) > maxRemoteStateBytes {
		return nil, fmt.Errorf("deck-service %s response exceeded %d bytes", remoteStatePath, maxRemoteStateBytes)
	}
	body, err := httpcoding.DecodeBody(resp.Header.Get("Content-Encoding"), raw, maxRemoteStateBytes)
	if err != nil {
		return nil, err
	}
	var state remoteDeckState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("deck-service %s: %w", remoteStatePath, err)
	}
	return &state, nil
}

// adoptRemoteReadiness skips pushes the target does not need: after a Cloud
// restart every target looks cold, but a deck-service that already holds this
// region's registry contentHash and the exact music metas (same sha256) is
// ready as it is. Music metas of a region the target pulls from the registry
// are never pushed at all (see registryOwnsMusicMetas). Anything unknown keeps
// the readiness flags as they were, so the caller pushes exactly as before.
func (r *RemoteDeckRecommender) adoptRemoteReadiness(ctx context.Context, exec *remoteExecution, region, musicHash string, masterReady, musicReady bool) (bool, bool) {
	remote, err := r.fetchRemoteDeckState(ctx, exec)
	if err != nil {
		r.logger.DebugContext(ctx, "deck-service state probe failed; pushing as before",
			"upstream", deckServiceName, "error_type", fmt.Sprintf("%T", err))
		return masterReady, musicReady
	}
	region = strings.ToLower(strings.TrimSpace(region))
	if !masterReady && r.registryURL != "" {
		if remoteHash := remote.contentHash(region); remoteHash != "" {
			if local := r.currentRegistryContentHash(); local == "" || local == remoteHash {
				r.adoptRegistryContentHash(remoteHash)
				masterReady = true
				commandtrace.RecordOperation(ctx, "deck.ready_masterdata_current", 0)
			}
		}
	}
	if !musicReady && musicHash != "" {
		switch {
		case !strings.HasPrefix(musicHash, musicMetaPathHashTag) && remote.musicMetasDigest(region) == musicHash:
			musicReady = true
			commandtrace.RecordOperation(ctx, "deck.ready_music_metas_current", 0)
		case remote.registryOwnsMusicMetas(region):
			r.revalidateRemoteRegistry(ctx, exec, region)
			musicReady = true
			commandtrace.RecordOperation(ctx, "deck.ready_music_metas_registry", 0)
		}
	}
	return masterReady, musicReady
}

// revalidateRemoteRegistry asks a registry-mode target to recheck the region
// against the registry now (manifest, then music metas, both conditional)
// instead of being handed Cloud's copy. Whichever of Cloud and the target is
// behind, the target ends up on the registry's current metas; one that is
// already current answers from 304s. A failure is only logged: the target
// keeps what it holds and catches up on its own refresh.
func (r *RemoteDeckRecommender) revalidateRemoteRegistry(ctx context.Context, exec *remoteExecution, region string) {
	var response remoteRegistryUpdateResponse
	if err := r.postJSON(ctx, exec, "/update/masterdata/registry", map[string]any{"region": region}, &response); err != nil {
		r.logger.WarnContext(ctx, "deck-service registry revalidation failed; the target keeps its music metas",
			"upstream", deckServiceName, "region", region, "error_type", fmt.Sprintf("%T", err))
		return
	}
	if r.registryURL != "" {
		r.adoptRegistryContentHash(response.ContentHash)
	}
}
