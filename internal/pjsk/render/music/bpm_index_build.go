package music

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"

	"haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
)

// BuildBPMIndex reads both complete regional score prefixes. A failed listing,
// download or parse aborts the build so an incomplete scan is never published
// as an authoritative missing-chart index. The caller supplies the resource
// revision only after its asset uploads have completed.
func BuildBPMIndex(ctx context.Context, store storage.Store, region, revision string) (*BPMIndex, error) {
	region, err := normalizeBPMIndexRegion(region)
	if err != nil {
		return nil, err
	}
	if store == nil || store == storage.Disabled() {
		return nil, storage.ErrNotConfigured
	}
	index := &BPMIndex{SchemaVersion: BPMIndexSchemaVersion, Region: region, ResourceRevision: revision, Complete: true, Prefixes: bpmIndexPrefixes(region), Charts: make(map[string]BPMIndexChart)}
	if err := index.validate(); err != nil {
		return nil, err
	}
	seen := make(map[storage.Key]struct{})
	var keys []storage.Key
	for _, prefix := range index.Prefixes {
		if err := store.List(ctx, storage.Key(prefix), func(object storage.Object) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !bpmIndexScoreKey(region, object.Key) {
				return nil
			}
			if _, ok := seen[object.Key]; ok {
				return nil
			}
			if len(keys) >= bpmIndexMaxCharts {
				return fmt.Errorf("BPM index exceeds %d charts", bpmIndexMaxCharts)
			}
			seen[object.Key] = struct{}{}
			keys = append(keys, object.Key)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("list BPM chart prefix: %w", err)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	charts := make([]BPMIndexChart, len(keys))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(8)
	for i, key := range keys {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			data, err := store.Get(groupCtx, key)
			if err != nil {
				return fmt.Errorf("read BPM chart %s: %w", key, err)
			}
			parsed, err := parseChartBPM(groupCtx, bytes.NewReader(data))
			if err != nil {
				return fmt.Errorf("parse BPM chart %s: %w", key, err)
			}
			charts[i] = indexedChart(parsed)
			return charts[i].validate()
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i, key := range keys {
		index.Charts[string(key)] = charts[i]
	}
	return index, nil
}

// PublishBPMIndex writes a content-addressed JSON object. Existing objects are
// verified and reused, never overwritten. Publish the returned reference in
// the resource manifest only after this function succeeds.
func PublishBPMIndex(ctx context.Context, store storage.Store, index *BPMIndex) (storage.Key, string, error) {
	if store == nil || store == storage.Disabled() {
		return "", "", storage.ErrNotConfigured
	}
	if err := index.validate(); err != nil {
		return "", "", err
	}
	data, err := jsonutil.Marshal(index)
	if err != nil {
		return "", "", err
	}
	if len(data) > bpmIndexMaxBytes {
		return "", "", fmt.Errorf("BPM index exceeds %d bytes", bpmIndexMaxBytes)
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	key := storage.Key("indexes/bpm/v1/" + index.Region + "/" + hash + ".json")
	existing, err := store.Get(ctx, key)
	if err == nil {
		if !bytes.Equal(existing, data) {
			return "", "", fmt.Errorf("existing BPM index content does not match its immutable key")
		}
		return key, hash, nil
	}
	if !errors.Is(err, storage.ErrNotExist) {
		return "", "", err
	}
	if err := store.Put(ctx, key, data, storage.PutOptions{ContentType: "application/json", CacheControl: "public,max-age=31536000,immutable"}); err != nil {
		return "", "", err
	}
	return key, hash, nil
}
