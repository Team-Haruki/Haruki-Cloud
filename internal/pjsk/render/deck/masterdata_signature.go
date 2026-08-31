package deck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
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
