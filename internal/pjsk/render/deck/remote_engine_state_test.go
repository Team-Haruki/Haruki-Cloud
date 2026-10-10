package deck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/httpcoding"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"github.com/klauspost/compress/zstd"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type fakeDeckState struct {
	stateStatus    int
	stateBody      string
	registryStatus int
	advertise      bool
	refuseZstd     bool
	zstdReply      bool

	mu              sync.Mutex
	musicPushes     int
	masterPushes    int
	stateProbes     int
	encodedBodies   int
	identityBodies  int
	lastMusicData   []byte
	registryBodies  [][]byte
	refusedRequests atomic.Int32
}

func (f *fakeDeckState) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if f.advertise {
			w.Header().Set("Accept-Encoding", "zstd")
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Encoding") == "zstd" {
			if f.refuseZstd {
				f.refusedRequests.Add(1)
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			decoded, err := httpcoding.DecodeBody("zstd", raw, 64<<20)
			if err != nil {
				t.Errorf("server could not decode body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			raw = decoded
			f.mu.Lock()
			f.encodedBodies++
			f.mu.Unlock()
		} else if r.Method == http.MethodPost {
			f.mu.Lock()
			f.identityBodies++
			f.mu.Unlock()
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case remoteStatePath:
			f.stateProbes++
			status := f.stateStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(f.stateBody))
		case "/update/musicmetas/string", "/update/musicmetas":
			f.musicPushes++
			f.lastMusicData = raw
			f.reply(w, `{"status":"ok"}`)
		case "/update/masterdata/registry", "/update/masterdata":
			f.masterPushes++
			if r.URL.Path == "/update/masterdata/registry" {
				f.registryBodies = append(f.registryBodies, raw)
				if f.registryStatus != 0 {
					w.WriteHeader(f.registryStatus)
					return
				}
			}
			f.reply(w, `{"status":"ok","contentHash":"remote-hash"}`)
		default:
			http.NotFound(w, r)
		}
	}
}

func (f *fakeDeckState) reply(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	if f.zstdReply {
		enc, _ := zstd.NewWriter(nil)
		w.Header().Set("Content-Encoding", "zstd")
		_, _ = w.Write(enc.EncodeAll([]byte(body), nil))
		return
	}
	_, _ = w.Write([]byte(body))
}

func newFakeDeckRecommender(t *testing.T, fake *fakeDeckState, registry bool) (*RemoteDeckRecommender, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)
	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.coding = httpcoding.NewNegotiator()
	recommender.logger = logger.NewLoggerFromGlobal("DeckRemoteTest")
	recommender.region = "jp"
	recommender.maxRetries = 0
	if registry {
		recommender.registryURL = "http://registry.invalid"
	}
	return recommender, server
}

func largeMusicMeta() []byte {
	return []byte(`[` + strings.Repeat(`{"music_id":10000,"difficulty":"master","music_time":123.4},`, 400) + `{"music_id":1}]`)
}

func TestEnsureReadySkipsPushWhenTargetHoldsSameMusicMetas(t *testing.T) {
	meta := largeMusicMeta()
	for _, tc := range []struct {
		name  string
		state string
	}{
		{"musicMetas map", `{"regions":{},"musicMetas":{"jp":"` + sha256Hex(meta) + `"}}`},
		{"older registry digest", `{"regions":{"jp":{"contentHash":"h","musicMetasDigest":"` + sha256Hex(meta) + `"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDeckState{stateBody: tc.state}
			recommender, _ := newFakeDeckRecommender(t, fake, false)
			ctx, trace := commandtrace.WithNewTrace(context.Background())
			exec := testRemoteExecution(t, recommender)
			defer exec.Release()
			if err := recommender.ensureReady(ctx, exec, "jp", meta, ""); err != nil {
				t.Fatalf("ensureReady() error = %v", err)
			}
			if fake.musicPushes != 0 || fake.stateProbes != 1 {
				t.Fatalf("pushes=%d probes=%d, want 0 pushes and 1 probe", fake.musicPushes, fake.stateProbes)
			}
			if _, ok := traceOperation(trace.Snapshot(), "deck.ready_music_metas_current"); !ok {
				t.Fatal("skip must be recorded in the trace")
			}
			// Ready now: a second call neither probes nor pushes.
			if err := recommender.ensureReady(ctx, exec, "jp", meta, ""); err != nil {
				t.Fatalf("repeat ensureReady() error = %v", err)
			}
			if fake.musicPushes != 0 || fake.stateProbes != 1 {
				t.Fatalf("repeat: pushes=%d probes=%d", fake.musicPushes, fake.stateProbes)
			}
		})
	}
}

func TestEnsureReadyPushesWhenTargetStateDiffersOrIsUnavailable(t *testing.T) {
	meta := largeMusicMeta()
	for _, tc := range []struct {
		name   string
		status int
		state  string
	}{
		{"different digest", 0, `{"musicMetas":{"jp":"` + sha256Hex([]byte("other")) + `"}}`},
		{"other region only", 0, `{"musicMetas":{"cn":"` + sha256Hex(meta) + `"}}`},
		{"old service without endpoint", http.StatusNotFound, `not found`},
		{"malformed state", 0, `{"musicMetas":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDeckState{stateStatus: tc.status, stateBody: tc.state}
			recommender, _ := newFakeDeckRecommender(t, fake, false)
			exec := testRemoteExecution(t, recommender)
			defer exec.Release()
			if err := recommender.ensureReady(context.Background(), exec, "jp", meta, ""); err != nil {
				t.Fatalf("ensureReady() error = %v", err)
			}
			if fake.musicPushes != 1 {
				t.Fatalf("expected one push, got %d", fake.musicPushes)
			}
			if !bytes.Contains(fake.lastMusicData, []byte(`"region":"jp"`)) {
				t.Fatalf("push body lacks region: %.80s", fake.lastMusicData)
			}
		})
	}
}

// A target that pulls a region from the registry keeps that region's metas
// current itself. Cloud's copy can lag the registry, and pushing it rolled the
// target back, so Cloud asks the target to revalidate against the registry
// instead of pushing.
func TestEnsureReadyNeverPushesMusicMetasToRegistryOwnedRegion(t *testing.T) {
	meta := largeMusicMeta()
	owned := `{"regions":{"jp":{"contentHash":"remote-hash","source":"registry","musicMetasDigest":"` + sha256Hex([]byte("registry copy")) + `"}},` +
		`"musicMetas":{"jp":"` + sha256Hex([]byte("registry copy")) + `"}}`

	for _, tc := range []struct {
		name           string
		meta           []byte
		path           string
		registry       bool
		registryStatus int
	}{
		{name: "string metas, registry mode", meta: meta, registry: true},
		{name: "string metas, directory mode", meta: meta},
		{name: "file-path metas", path: "/data/metas.json", registry: true},
		{name: "revalidation fails", meta: meta, registry: true, registryStatus: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDeckState{stateBody: owned, registryStatus: tc.registryStatus}
			recommender, _ := newFakeDeckRecommender(t, fake, tc.registry)
			ctx, trace := commandtrace.WithNewTrace(context.Background())
			exec := testRemoteExecution(t, recommender)
			defer exec.Release()
			if err := recommender.ensureReady(ctx, exec, "jp", tc.meta, tc.path); err != nil {
				t.Fatalf("ensureReady() error = %v", err)
			}
			if fake.musicPushes != 0 {
				t.Fatalf("music metas pushed %d times to a registry-owned region", fake.musicPushes)
			}
			if len(fake.registryBodies) != 1 {
				t.Fatalf("registry revalidations = %d, want 1", len(fake.registryBodies))
			}
			body := string(fake.registryBodies[0])
			if !strings.Contains(body, `"region":"jp"`) || strings.Contains(body, "content_hash") {
				t.Fatalf("revalidation must name only the region, got %s", body)
			}
			if _, ok := traceOperation(trace.Snapshot(), "deck.ready_music_metas_registry"); !ok {
				t.Fatal("registry ownership must be recorded in the trace")
			}
			// Ready for this copy now: the next call neither probes nor pushes.
			if err := recommender.ensureReady(ctx, exec, "jp", tc.meta, tc.path); err != nil {
				t.Fatalf("repeat ensureReady() error = %v", err)
			}
			if fake.stateProbes != 1 || fake.musicPushes != 0 || len(fake.registryBodies) != 1 {
				t.Fatalf("repeat: probes=%d pushes=%d revalidations=%d", fake.stateProbes, fake.musicPushes, len(fake.registryBodies))
			}
		})
	}

	t.Run("region without registry metas is still pushed", func(t *testing.T) {
		fake := &fakeDeckState{stateBody: `{"regions":{"jp":{"contentHash":"remote-hash","source":"registry"}}}`}
		recommender, _ := newFakeDeckRecommender(t, fake, true)
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.ensureReady(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("ensureReady() error = %v", err)
		}
		if fake.musicPushes != 1 || len(fake.registryBodies) != 0 {
			t.Fatalf("pushes=%d revalidations=%d, want 1 and 0", fake.musicPushes, len(fake.registryBodies))
		}
	})

	t.Run("revalidation adopts the target's registry hash", func(t *testing.T) {
		fake := &fakeDeckState{stateBody: owned}
		recommender, _ := newFakeDeckRecommender(t, fake, true)
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.ensureReady(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("ensureReady() error = %v", err)
		}
		if got := recommender.currentRegistryContentHash(); got != "remote-hash" {
			t.Fatalf("registry hash = %q, want remote-hash", got)
		}
	})
}

func TestEnsureReadyNeverSkipsFilePathMusicMetas(t *testing.T) {
	fake := &fakeDeckState{stateBody: `{"musicMetas":{"jp":"path:/data/metas.json"}}`}
	recommender, _ := newFakeDeckRecommender(t, fake, false)
	exec := testRemoteExecution(t, recommender)
	defer exec.Release()
	if err := recommender.ensureReady(context.Background(), exec, "jp", nil, "/data/metas.json"); err != nil {
		t.Fatalf("ensureReady() error = %v", err)
	}
	if fake.musicPushes != 1 {
		t.Fatalf("a file-path push has no digest to compare and must be sent, got %d", fake.musicPushes)
	}
}

func TestEnsureReadyAdoptsCurrentRegistryMasterdata(t *testing.T) {
	meta := largeMusicMeta()
	state := `{"regions":{"jp":{"contentHash":"remote-hash"}},"musicMetas":{"jp":"` + sha256Hex(meta) + `"}}`

	t.Run("matching or unknown local hash", func(t *testing.T) {
		fake := &fakeDeckState{stateBody: state}
		recommender, _ := newFakeDeckRecommender(t, fake, true)
		target := testRemoteTargetState(t, recommender)
		target.masterdataReady = false
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.ensureReady(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("ensureReady() error = %v", err)
		}
		if fake.masterPushes != 0 || fake.musicPushes != 0 {
			t.Fatalf("masterPushes=%d musicPushes=%d, want none", fake.masterPushes, fake.musicPushes)
		}
		if got := recommender.currentRegistryContentHash(); got != "remote-hash" {
			t.Fatalf("registry hash = %q, want adopted remote-hash", got)
		}
	})

	t.Run("stale remote masterdata", func(t *testing.T) {
		fake := &fakeDeckState{stateBody: state}
		recommender, _ := newFakeDeckRecommender(t, fake, true)
		recommender.registryContentHash = "newer-hash"
		target := testRemoteTargetState(t, recommender)
		target.masterdataReady = false
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.ensureReady(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("ensureReady() error = %v", err)
		}
		if fake.masterPushes != 1 {
			t.Fatalf("stale masterdata must be reloaded, got %d", fake.masterPushes)
		}
	})

	t.Run("directory mode always pushes masterdata", func(t *testing.T) {
		fake := &fakeDeckState{stateBody: state}
		recommender, _ := newFakeDeckRecommender(t, fake, false)
		recommender.masterdataDir = "/masterdata"
		target := testRemoteTargetState(t, recommender)
		target.masterdataReady = false
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.ensureReady(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("ensureReady() error = %v", err)
		}
		if fake.masterPushes != 1 || fake.musicPushes != 0 {
			t.Fatalf("masterPushes=%d musicPushes=%d", fake.masterPushes, fake.musicPushes)
		}
	})
}

func TestDeckPostNegotiatesZstdBodies(t *testing.T) {
	meta := largeMusicMeta()

	t.Run("identity until advertised, then zstd", func(t *testing.T) {
		fake := &fakeDeckState{advertise: true, zstdReply: true, stateBody: `{}`}
		recommender, server := newFakeDeckRecommender(t, fake, false)
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.updateRemoteMusicMeta(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("first push error = %v", err)
		}
		if fake.identityBodies != 1 || fake.encodedBodies != 0 {
			t.Fatalf("first push must be identity: identity=%d encoded=%d", fake.identityBodies, fake.encodedBodies)
		}
		if !recommender.coding.Supports(server.URL) {
			t.Fatal("the advertisement must be learned")
		}
		var response remoteRegistryUpdateResponse
		if err := recommender.postJSON(context.Background(), exec, "/update/musicmetas/string", map[string]any{"region": "jp", "data": string(meta)}, &response); err != nil {
			t.Fatalf("second push error = %v", err)
		}
		if fake.encodedBodies != 1 {
			t.Fatalf("second push must be zstd, encoded=%d", fake.encodedBodies)
		}
		if !bytes.Contains(fake.lastMusicData, []byte(`music_time`)) {
			t.Fatal("server must see the decoded body")
		}
		if response.Status != "ok" {
			t.Fatalf("zstd response not decoded: %+v", response)
		}
	})

	t.Run("415 withdraws zstd and resends identity", func(t *testing.T) {
		fake := &fakeDeckState{advertise: true, refuseZstd: true, stateBody: `{}`}
		recommender, server := newFakeDeckRecommender(t, fake, false)
		recommender.coding.Observe(server.URL, http.Header{"Accept-Encoding": {"zstd"}})
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		if err := recommender.updateRemoteMusicMeta(context.Background(), exec, "jp", meta, ""); err != nil {
			t.Fatalf("push error = %v", err)
		}
		if fake.refusedRequests.Load() != 1 || fake.identityBodies != 1 || fake.musicPushes != 1 {
			t.Fatalf("refused=%d identity=%d pushes=%d", fake.refusedRequests.Load(), fake.identityBodies, fake.musicPushes)
		}
	})

	t.Run("octet-stream payloads are not encoded twice", func(t *testing.T) {
		fake := &fakeDeckState{advertise: true, stateBody: `{}`}
		recommender, server := newFakeDeckRecommender(t, fake, false)
		recommender.coding.Observe(server.URL, http.Header{"Accept-Encoding": {"zstd"}})
		exec := testRemoteExecution(t, recommender)
		defer exec.Release()
		_ = recommender.postBinary(context.Background(), exec, "/cache_userdata", bytes.Repeat([]byte{1}, 8<<10), nil)
		if fake.encodedBodies != 0 || fake.identityBodies != 1 {
			t.Fatalf("binary body must stay as is: encoded=%d identity=%d", fake.encodedBodies, fake.identityBodies)
		}
	})

	t.Run("health checks learn the advertisement", func(t *testing.T) {
		fake := &fakeDeckState{advertise: true, stateBody: `{}`}
		recommender, server := newFakeDeckRecommender(t, fake, false)
		_ = recommender.healthCheck(context.Background(), server.URL)
		if !recommender.coding.Supports(server.URL) {
			t.Fatal("health check must record the advertisement")
		}
	})
}

func TestDeckPostRejectsUndecodableResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "zstd")
		_, _ = w.Write([]byte("not a zstd frame"))
	}))
	defer server.Close()
	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.logger = logger.NewLoggerFromGlobal("DeckRemoteTest")
	exec := testRemoteExecution(t, recommender)
	defer exec.Release()
	var out map[string]any
	if err := recommender.postJSON(context.Background(), exec, "/update/musicmetas/string", map[string]any{"region": "jp"}, &out); err == nil {
		t.Fatal("an invalid zstd response must fail")
	}
}

func TestRemoteDeckStateAccessorsTolerateNil(t *testing.T) {
	var state *remoteDeckState
	if state.musicMetasDigest("jp") != "" || state.contentHash("jp") != "" || state.registryOwnsMusicMetas("jp") {
		t.Fatal("nil state reports nothing")
	}
	var recommender *RemoteDeckRecommender
	if _, err := recommender.fetchRemoteDeckState(context.Background(), &remoteExecution{}); err == nil {
		t.Fatal("unconfigured recommender cannot probe")
	}
}
