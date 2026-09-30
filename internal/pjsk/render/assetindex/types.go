// Package assetindex publishes and consumes complete, immutable asset inventories.
package assetindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
)

const Version = 1
const Root = "indexes/assets/v1/"

var Regions = []string{"jp", "en", "tw", "kr", "cn"}

type Object struct {
	Key      storage.Key `json:"key"`
	Size     int64       `json:"size"`
	ETag     string      `json:"etag,omitempty"`
	Modified time.Time   `json:"modified,omitempty"`
}
type Blob struct {
	Key      storage.Key `json:"key"`
	SHA256   string      `json:"sha256"`
	Revision string      `json:"revision,omitempty"`
}
type ShardRef struct {
	Prefix string `json:"prefix"`
	Blob
}
type Manifest struct {
	Version        int        `json:"version"`
	Region         string     `json:"region"`
	Revision       string     `json:"revision"`
	Complete       bool       `json:"complete"`
	PublishedAt    time.Time  `json:"published_at"`
	Shards         []ShardRef `json:"shards"`
	BPM            *Blob      `json:"bpm,omitempty"`
	sourceRevision string
}
type Shard struct {
	Version int      `json:"version"`
	Region  string   `json:"region"`
	Prefix  string   `json:"prefix"`
	Objects []Object `json:"objects"`
}

type Config struct {
	Enabled      bool
	PollInterval time.Duration
	Timeout      time.Duration
	MaxStale     time.Duration
	MaxObjects   int
}

func (c Config) defaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Minute
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxStale <= 0 {
		c.MaxStale = 24 * time.Hour
	}
	if c.MaxObjects <= 0 {
		c.MaxObjects = 500_000
	}
	return c
}
func validRegion(region string) bool {
	for _, r := range Regions {
		if region == r {
			return true
		}
	}
	return false
}
func PointerKey(region string) storage.Key { return storage.Key(Root + region + "/current.json") }
func digest(data []byte) string            { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func validateBlob(b Blob, prefix string) error {
	k, err := storage.CleanKey(string(b.Key))
	if err != nil || k != b.Key || !strings.HasPrefix(string(k), prefix) || !validDigest(b.SHA256) {
		return fmt.Errorf("asset index: invalid blob reference")
	}
	return nil
}
func readBlob(ctx context.Context, store storage.Store, b Blob) ([]byte, error) {
	data, err := store.Get(ctx, b.Key)
	if err != nil {
		return nil, err
	}
	if digest(data) != b.SHA256 {
		return nil, fmt.Errorf("asset index: content digest mismatch")
	}
	return data, nil
}
func writeBlob(ctx context.Context, store storage.Store, prefix string, value any) (Blob, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Blob{}, err
	}
	hash := digest(data)
	b := Blob{Key: storage.Key(prefix + hash + ".json"), SHA256: hash}
	// Rewriting the same content-addressed key is idempotent; readers verify its digest.
	err = store.Put(ctx, b.Key, data, storage.PutOptions{ContentType: "application/json"})
	return b, err
}

func manifestRevision(shards []ShardRef) string {
	refs := append([]ShardRef(nil), shards...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].Prefix < refs[j].Prefix })
	var b strings.Builder
	for _, ref := range refs {
		b.WriteString(ref.Prefix)
		b.WriteByte(0)
		b.WriteString(ref.SHA256)
		b.WriteByte(10)
	}
	return digest([]byte(b.String()))
}
