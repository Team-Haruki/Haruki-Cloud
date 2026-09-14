package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/s3"
)

// TestGarageIntegration is the opt-in round trip against a real Garage node:
//
//	HARUKI_RUN_INTEGRATION=1 \
//	HARUKI_S3_TEST_ENDPOINTS=http://100.64.0.11:3900 HARUKI_S3_TEST_BUCKET=... \
//	HARUKI_S3_TEST_ACCESS_KEY_ID=... HARUKI_S3_TEST_SECRET_ACCESS_KEY=... \
//	[HARUKI_S3_TEST_PUBLIC_BASE_URL=https://image-cache.node.example] \
//	go test ./internal/storage/s3/... -run Garage
func TestGarageIntegration(t *testing.T) {
	if os.Getenv("HARUKI_RUN_INTEGRATION") != "1" {
		t.Skip("set HARUKI_RUN_INTEGRATION=1 to run against a real Garage node")
	}
	endpoints := strings.Split(os.Getenv("HARUKI_S3_TEST_ENDPOINTS"), ",")
	store, err := s3.New(s3.Config{
		Endpoints:  endpoints,
		Bucket:     os.Getenv("HARUKI_S3_TEST_BUCKET"),
		Root:       "haruki-cloud-integration",
		AccessKey:  os.Getenv("HARUKI_S3_TEST_ACCESS_KEY_ID"),
		SecretKey:  os.Getenv("HARUKI_S3_TEST_SECRET_ACCESS_KEY"),
		PathStyle:  true,
		DefaultACL: os.Getenv("HARUKI_S3_TEST_DEFAULT_ACL"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	key := storage.Key("run-" + time.Now().UTC().Format("20060102T150405.000000000") + "/a b+ü.txt")
	payload := []byte("haruki garage integration")
	if err := store.Put(ctx, key, payload, storage.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })
	if got, err := store.Get(ctx, key); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if object, err := store.Stat(ctx, key); err != nil || object.Size != int64(len(payload)) {
		t.Fatalf("Stat = %+v, %v", object, err)
	}
	found := false
	if err := store.List(ctx, key[:strings.Index(string(key), "/")+1], func(object storage.Object) error {
		found = found || object.Key == key
		return nil
	}); err != nil || !found {
		t.Fatalf("List found=%v err=%v", found, err)
	}
	if base := os.Getenv("HARUKI_S3_TEST_PUBLIC_BASE_URL"); base != "" {
		publicGet(ctx, t, strings.TrimRight(base, "/")+"/haruki-cloud-integration/"+string(key), payload)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("Get after Delete error = %v", err)
	}
}

func publicGet(ctx context.Context, t *testing.T, rawURL string, want []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got, want) {
		t.Fatalf("public GET status=%d body=%q", resp.StatusCode, got)
	}
}
