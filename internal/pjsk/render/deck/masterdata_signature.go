package deck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
			r.logger.Warnf("failed to capture masterdata signature: %v", err)
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
			r.logger.Warnf("failed to check masterdata update: %v", err)
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
		r.logger.Infof("masterdata changed; will refresh deck-service on next request (region=%s dir=%s files=%d)", r.region, signature.Dir, signature.Files)
	}
}

func deckMasterdataDirSignature(configured, region string) (deckMasterdataSignature, error) {
	dir, ok := resolveDeckMasterdataContentDir(configured, region)
	if !ok {
		return deckMasterdataSignature{}, fmt.Errorf("masterdata dir not found for region %s under %s", strings.TrimSpace(region), strings.TrimSpace(configured))
	}

	hasher := sha256.New()
	files := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
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
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}
		files++
		fmt.Fprintf(hasher, "%s\x00%d\x00%d\x00", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return deckMasterdataSignature{}, err
	}
	if files == 0 {
		return deckMasterdataSignature{}, fmt.Errorf("masterdata dir %s has no json files", dir)
	}
	return deckMasterdataSignature{
		Dir:   dir,
		Hash:  hex.EncodeToString(hasher.Sum(nil)),
		Files: files,
	}, nil
}
