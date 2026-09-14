package imagecache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreAndGetURLSkipsExistingContentFile(t *testing.T) {
	dir := t.TempDir()
	client := New("https://images.example.test/", dir)
	data := []byte("rendered image")

	ctx, trace := commandtrace.WithTrace(context.Background())
	url, err := client.StoreAndGetURL(ctx, data, "pjsk/profile")
	if err != nil {
		t.Fatalf("StoreAndGetURL() error = %v", err)
	}

	digest := sha256.Sum256(data)
	name := hex.EncodeToString(digest[:]) + ".png"
	target := filepath.Join(dir, "pjsk", "profile", name)
	wantURL := "https://images.example.test/pjsk/profile/" + name
	if url != wantURL {
		t.Fatalf("StoreAndGetURL() URL = %q, want %q", url, wantURL)
	}
	if got, readErr := os.ReadFile(target); readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	} else if string(got) != string(data) {
		t.Fatalf("stored data = %q, want %q", got, data)
	}
	assertTraceOperation(t, trace, "image.hash")
	assertTraceOperation(t, trace, "image.lookup")
	assertTraceOperation(t, trace, "image.write")

	oldTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	secondCtx, secondTrace := commandtrace.WithTrace(context.Background())
	if _, err := client.StoreAndGetURL(secondCtx, data, "pjsk/profile"); err != nil {
		t.Fatalf("second StoreAndGetURL() error = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatalf("existing content file was rewritten: mtime = %v, want %v", info.ModTime(), oldTime)
	}
	assertTraceOperation(t, secondTrace, "image.hash")
	assertTraceOperation(t, secondTrace, "image.lookup")
	assertNoTraceOperation(t, secondTrace, "image.write")

	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".*.tmp-*")); err != nil {
		t.Fatalf("Glob() error = %v", err)
	} else if len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", matches)
	}
}

func TestNormalizeImageGroup(t *testing.T) {
	tests := []struct {
		name    string
		group   string
		want    string
		wantErr bool
	}{
		{name: "nested", group: " pjsk/profile ", want: filepath.Join("pjsk", "profile")},
		{name: "cleans segments", group: "pjsk/tmp/../card", want: filepath.Join("pjsk", "card")},
		{name: "empty", group: " ", wantErr: true},
		{name: "absolute", group: "/tmp/images", wantErr: true},
		{name: "parent", group: "..", wantErr: true},
		{name: "parent prefix", group: "../outside", wantErr: true},
		{name: "nested escape", group: "pjsk/../../outside", wantErr: true},
		{name: "windows escape", group: `..\outside`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeImageGroup(test.group)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeImageGroup(%q) error = %v, wantErr %v", test.group, err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("normalizeImageGroup(%q) = %q, want %q", test.group, got, test.want)
			}
		})
	}
}

func TestStoreAndGetURLSingleflightSurvivesLeaderCancellation(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	data := []byte("shared rendered image")
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeCalls atomic.Int32
	client.objects = &hookStore{Store: client.objects, beforePut: func() {
		if writeCalls.Add(1) == 1 {
			close(writeStarted)
		}
		<-releaseWrite
	}}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := client.StoreAndGetURL(leaderCtx, data, "pjsk/shared")
		leaderDone <- err
	}()
	<-writeStarted

	followerDone := make(chan error, 1)
	followerBaseCtx, followerTrace := commandtrace.WithTrace(context.Background())
	followerCtx := &doneObservedContext{
		Context:  followerBaseCtx,
		observed: make(chan struct{}),
	}
	go func() {
		_, err := client.StoreAndGetURL(followerCtx, data, "pjsk/shared")
		followerDone <- err
	}()
	<-followerCtx.observed
	cancelLeader()
	if err := <-leaderDone; err != context.Canceled {
		t.Fatalf("leader error = %v, want context.Canceled", err)
	}
	close(releaseWrite)
	if err := <-followerDone; err != nil {
		t.Fatalf("follower StoreAndGetURL() error = %v", err)
	}
	if got := writeCalls.Load(); got != 1 {
		t.Fatalf("write calls = %d, want 1", got)
	}
	assertTraceOperation(t, followerTrace, "image.write")
	assertTraceOperation(t, followerTrace, "image.shared")
}

type doneObservedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestStoreAndGetURLConcurrentColdWritePublishesOnce(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	data := []byte("same image for every caller")
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeCalls atomic.Int32
	client.objects = &hookStore{Store: client.objects, beforePut: func() {
		if writeCalls.Add(1) == 1 {
			close(writeStarted)
		}
		<-releaseWrite
	}}

	const callers = 16
	start := make(chan struct{})
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			_, err := client.StoreAndGetURL(context.Background(), data, "pjsk/concurrent")
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	<-writeStarted
	close(releaseWrite)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("StoreAndGetURL() error = %v", err)
		}
	}
	if got := writeCalls.Load(); got != 1 {
		t.Fatalf("write calls = %d, want 1", got)
	}
}

func TestStoreAndGetURLOwnsDataAfterCallerCancellation(t *testing.T) {
	client := New("https://images.example.test", t.TempDir())
	data := bytes.Repeat([]byte("caller-owned-image"), 1<<12)
	want := bytes.Clone(data)
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	writeDone := make(chan error, 1)
	hooked := &hookStore{Store: client.objects, beforePut: func() {
		close(writeStarted)
		<-releaseWrite
	}}
	hooked.afterPut = func(err error) { writeDone <- err }
	client.objects = hooked

	ctx, cancel := context.WithCancel(context.Background())
	callerDone := make(chan error, 1)
	go func() {
		_, err := client.StoreAndGetURL(ctx, data, "pjsk/owned")
		callerDone <- err
	}()
	<-writeStarted
	cancel()
	if err := <-callerDone; err != context.Canceled {
		t.Fatalf("StoreAndGetURL() error = %v, want context.Canceled", err)
	}

	for i := range data {
		data[i] ^= 0xff
	}
	close(releaseWrite)
	if err := <-writeDone; err != nil {
		t.Fatalf("detached write error = %v", err)
	}
	if _, err := client.StoreAndGetURL(context.Background(), want, "pjsk/owned"); err != nil {
		t.Fatalf("wait for detached StoreAndGetURL() error = %v", err)
	}

	digest := sha256.Sum256(want)
	target := filepath.Join(client.localRoot, "pjsk", "owned", hex.EncodeToString(digest[:])+".png")
	stored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(stored, want) {
		t.Fatal("stored content changed after caller mutated its source slice")
	}
}

func assertTraceOperation(t *testing.T, trace *commandtrace.Trace, name string) {
	t.Helper()
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name && operation.Count > 0 {
			return
		}
	}
	t.Fatalf("trace operation %q was not recorded: %+v", name, trace.Snapshot().Operations)
}

func assertNoTraceOperation(t *testing.T, trace *commandtrace.Trace, name string) {
	t.Helper()
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name && operation.Count > 0 {
			t.Fatalf("trace operation %q unexpectedly recorded: %+v", name, trace.Snapshot().Operations)
		}
	}
}

// hookStore wraps a Store to block, fail or record Put calls.
type hookStore struct {
	storage.Store
	beforePut func()
	afterPut  func(error)
	putErr    error
	statErr   error

	mu      sync.Mutex
	putOpts []storage.PutOptions
}

func (h *hookStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	h.mu.Lock()
	h.putOpts = append(h.putOpts, opts)
	h.mu.Unlock()
	if h.beforePut != nil {
		h.beforePut()
	}
	err := h.putErr
	if err == nil {
		err = h.Store.Put(ctx, key, data, opts)
	}
	if h.afterPut != nil {
		h.afterPut(err)
	}
	return err
}

func (h *hookStore) Stat(ctx context.Context, key storage.Key) (storage.Object, error) {
	if h.statErr != nil {
		return storage.Object{}, h.statErr
	}
	return h.Store.Stat(ctx, key)
}

func (h *hookStore) options() []storage.PutOptions {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]storage.PutOptions(nil), h.putOpts...)
}

func testHosts(t *testing.T) *urlhost.Set {
	t.Helper()
	hosts, err := urlhost.New(map[string]string{"cn09": "https://ic-cn09.example"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return hosts
}

func contentName(data []byte, ext string) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) + ext
}

func widenedMockStore(t *testing.T) (*PGStore, sqlmock.Sqlmock) {
	t.Helper()
	store, mock := newMockPGStore(t)
	store.widened.Store(true)
	return store, mock
}

var widenedLookupColumns = []string{"cdn_path", "file_path", "size_bytes", "storage_backend", "media_type", "expires_at"}

func TestNewClientRejectsMissingHostsOrObjects(t *testing.T) {
	memory := storagetest.NewMemory()
	for _, tc := range []struct {
		name string
		cfg  ClientConfig
		want error
	}{
		{"nil hosts", ClientConfig{Objects: memory}, ErrNoHosts},
		{"empty hosts", ClientConfig{Hosts: urlhost.Single(""), Objects: memory}, ErrNoHosts},
		{"nil objects", ClientConfig{Hosts: testHosts(t)}, ErrNoObjectStore},
		{"disabled objects", ClientConfig{Hosts: testHosts(t), Objects: storage.Disabled()}, ErrNoObjectStore},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(tc.cfg)
			if client != nil || !errors.Is(err, tc.want) {
				t.Fatalf("NewClient() = %v, %v; want %v", client, err, tc.want)
			}
		})
	}
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: memory, LocalRoot: " "})
	if err != nil || client == nil || client.localRoot != "" {
		t.Fatalf("NewClient(valid) = %+v, %v", client, err)
	}
	if New("not a url", t.TempDir()) != nil {
		t.Fatal("invalid legacy uri built a client")
	}
}

func TestStoreHashedGarageSlotWritesObjectAndIndexesGarageRow(t *testing.T) {
	data := []byte("garage image")
	name := contentName(data, ".png")
	memory := storagetest.NewMemory()
	index, mock := widenedMockStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(lookupWidenedSQL)).WithArgs(contentName(data, "")).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(insertWidenedSQL)).
		WithArgs(contentName(data, ""), "pjsk", "pjsk/"+name, sql.NullString{}, int64(len(data)), BackendGarage, sql.NullString{String: "image/png", Valid: true}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: memory, Index: index})
	if err != nil {
		t.Fatal(err)
	}
	url, err := client.StoreAndGetURL(context.Background(), data, "pjsk")
	if err != nil || url != "https://ic-cn09.example/pjsk/"+name {
		t.Fatalf("StoreAndGetURL() = %q, %v", url, err)
	}
	if got, err := memory.Get(context.Background(), storage.Key("pjsk/"+name)); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("stored object = %q, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if url, ok := client.URLForFile(context.Background(), "/anything.png"); ok {
		t.Fatalf("URLForFile on a non-local slot = %q", url)
	}
}

func TestStoreHashedSkipsLocalStatForGarageRows(t *testing.T) {
	data := []byte("indexed garage image")
	hash := contentName(data, "")
	memory := storagetest.NewMemory()
	index, mock := widenedMockStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(lookupWidenedSQL)).WithArgs(hash).WillReturnRows(
		sqlmock.NewRows(widenedLookupColumns).AddRow("pjsk/api/x/"+hash+".png", "", int64(3), BackendGarage, "image/png", nil),
	)
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs(hash).WillReturnResult(sqlmock.NewResult(0, 1))
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: memory, LocalRoot: t.TempDir(), Index: index})
	if err != nil {
		t.Fatal(err)
	}
	url, err := client.StoreAndGetURL(context.Background(), data, "pjsk")
	if err != nil || url != "https://ic-cn09.example/pjsk/api/x/"+hash+".png" {
		t.Fatalf("StoreAndGetURL() = %q, %v", url, err)
	}
	if calls := memory.Calls(); len(calls) != 0 {
		t.Fatalf("garage row touched the object store: %+v", calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHashedProbesLegacyDiskRowsOnLocalSlot(t *testing.T) {
	data := []byte("legacy image")
	hash := contentName(data, "")
	name := contentName(data, ".png")
	legacyColumns := []string{"cdn_path", "file_path", "size_bytes"}

	t.Run("present object is trusted", func(t *testing.T) {
		memory := storagetest.NewMemory()
		memory.Seed(map[string][]byte{"old/" + name: data})
		index, mock := newMockPGStore(t)
		mock.ExpectQuery(regexp.QuoteMeta(lookupSQL)).WithArgs(hash).WillReturnRows(
			sqlmock.NewRows(legacyColumns).AddRow("old/"+name, "/cache/old/"+name, int64(len(data))),
		)
		client, _ := NewClient(ClientConfig{Hosts: testHosts(t), Objects: memory, LocalRoot: "/cache", Index: index})
		url, err := client.StoreAndGetURL(context.Background(), data, "pjsk")
		if err != nil || url != "https://ic-cn09.example/old/"+name {
			t.Fatalf("StoreAndGetURL() = %q, %v", url, err)
		}
		if calls := memory.Calls(); len(calls) != 1 || calls[0].Method != "Stat" {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("missing object is rewritten and reindexed", func(t *testing.T) {
		memory := storagetest.NewMemory()
		index, mock := newMockPGStore(t)
		mock.ExpectQuery(regexp.QuoteMeta(lookupSQL)).WithArgs(hash).WillReturnRows(
			sqlmock.NewRows(legacyColumns).AddRow("old/"+name, "/cache/old/"+name, int64(len(data))),
		)
		mock.ExpectExec(regexp.QuoteMeta(insertSQL)).
			WithArgs(hash, "pjsk", "pjsk/"+name, filepath.Join("/cache", "pjsk", name), int64(len(data))).
			WillReturnResult(sqlmock.NewResult(1, 1))
		client, _ := NewClient(ClientConfig{Hosts: testHosts(t), Objects: memory, LocalRoot: "/cache", Index: index})
		url, err := client.StoreAndGetURL(context.Background(), data, "pjsk")
		if err != nil || url != "https://ic-cn09.example/pjsk/"+name {
			t.Fatalf("StoreAndGetURL() = %q, %v", url, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoreHashedSendsNoCacheControl(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want string
	}{
		{[]byte("png-ish"), "image/png"},
		{[]byte{0xff, 0xd8, 0xff, 0xdb}, "image/jpeg"},
		{[]byte("GIF89a"), "image/gif"},
		{[]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp"},
	} {
		hooked := &hookStore{Store: storagetest.NewMemory()}
		client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: hooked})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.StoreAndGetURL(context.Background(), tc.data, "pjsk"); err != nil {
			t.Fatal(err)
		}
		opts := hooked.options()
		if len(opts) != 1 || opts[0].CacheControl != "" || opts[0].ContentType != tc.want {
			t.Fatalf("put options = %+v, want content type %q and no Cache-Control", opts, tc.want)
		}
	}
}

func TestStoreHashedInsertsOnlyAfterPut(t *testing.T) {
	data := []byte("never indexed")
	index, mock := newMockPGStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(lookupSQL)).WithArgs(contentName(data, "")).WillReturnError(sql.ErrNoRows)
	hooked := &hookStore{Store: storagetest.NewMemory(), putErr: errors.New("quorum lost")}
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: hooked, Index: index})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.StoreAndGetURL(context.Background(), data, "pjsk"); err == nil || !strings.Contains(err.Error(), "quorum lost") {
		t.Fatalf("StoreAndGetURL() error = %v", err)
	}
	// No INSERT expectation was registered: sqlmock fails any unexpected Exec.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHashedIndexErrorsAreLoggedNotFatal(t *testing.T) {
	data := []byte("index trouble")
	name := contentName(data, ".png")
	index, mock := newMockPGStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(lookupSQL)).WithArgs(contentName(data, "")).WillReturnError(errors.New("driver down"))
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WillReturnError(errors.New("insert down"))
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: storagetest.NewMemory(), Index: index})
	if err != nil {
		t.Fatal(err)
	}
	url, err := client.StoreAndGetURL(context.Background(), data, "pjsk")
	if err != nil || url != "https://ic-cn09.example/pjsk/"+name {
		t.Fatalf("StoreAndGetURL() = %q, %v", url, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHashedStatErrorIsReturned(t *testing.T) {
	hooked := &hookStore{Store: storagetest.NewMemory(), statErr: errors.New("stat exploded")}
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: hooked})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.StoreAndGetURL(context.Background(), []byte("x"), "pjsk"); err == nil || !strings.Contains(err.Error(), "stat exploded") {
		t.Fatalf("StoreAndGetURL() error = %v", err)
	}
}

func TestMediaTypeFromPath(t *testing.T) {
	for name, want := range map[string]string{
		"a.png": "image/png", "a.JPG": "image/jpeg", "a.jpeg": "image/jpeg",
		"a.gif": "image/gif", "a.webp": "image/webp", "a": "image/png",
	} {
		if got := mediaTypeFromPath(name); got != want {
			t.Fatalf("mediaTypeFromPath(%q) = %q, want %q", name, got, want)
		}
	}
}
