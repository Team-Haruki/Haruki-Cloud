package s3_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/s3"
	"haruki-cloud/internal/storage/s3/s3test"
	"haruki-cloud/internal/storage/storagetest"
)

const (
	testAccessKey = "GKconformance"
	testSecretKey = "conformance-secret"
)

func newFake(t *testing.T) *s3test.Server {
	t.Helper()
	server := s3test.New(s3test.Options{Bucket: "bucket", AccessKey: testAccessKey, SecretKey: testSecretKey})
	t.Cleanup(server.Close)
	return server
}

func newStore(t *testing.T, server *s3test.Server, mutate func(*s3.Config)) storage.Store {
	t.Helper()
	cfg := s3.Config{
		Endpoints:      []string{server.URL},
		Bucket:         "bucket",
		AccessKey:      testAccessKey,
		SecretKey:      testSecretKey,
		PathStyle:      true,
		MaxObjectBytes: storagetest.ConformanceMaxBytes,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	store, err := s3.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestConformanceAgainstS3Test(t *testing.T) {
	storagetest.Conformance(t, func(t *testing.T) storage.Store {
		return newStore(t, newFake(t), nil)
	})
}

func TestConformanceAgainstS3TestWithRoot(t *testing.T) {
	storagetest.Conformance(t, func(t *testing.T) storage.Store {
		return newStore(t, newFake(t), func(cfg *s3.Config) { cfg.Root = "/cloud/cache/" })
	})
}

func TestRootedObjectsLandUnderRoot(t *testing.T) {
	server := newFake(t)
	store := newStore(t, server, func(cfg *s3.Config) {
		cfg.Root = "pjsk"
		cfg.DefaultACL = "public-read"
	})
	if err := store.Put(context.Background(), "a b/ü+.json", []byte(`{}`), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	object, ok := server.Object("pjsk/a b/ü+.json")
	if !ok || object.ACL != "public-read" || object.CacheControl != "" || object.ContentType == "" {
		t.Fatalf("stored object = %+v, %v", object, ok)
	}
}

func TestSignatureMismatchIsRejected(t *testing.T) {
	server := newFake(t)
	store := newStore(t, server, func(cfg *s3.Config) { cfg.SecretKey = "wrong" })
	var respErr *s3.ResponseError
	err := store.Put(context.Background(), "k", []byte("x"), storage.PutOptions{})
	if !errors.As(err, &respErr) || respErr.StatusCode != http.StatusForbidden || respErr.Code != "SignatureDoesNotMatch" {
		t.Fatalf("Put error = %v", err)
	}
	if len(server.Requests()) != 1 {
		t.Fatal("a 403 must not be retried")
	}
}

func TestInjectedFailuresFailOver(t *testing.T) {
	first, second := newFake(t), newFake(t)
	store := newStore(t, first, func(cfg *s3.Config) { cfg.Endpoints = []string{first.URL, second.URL} })
	first.FailNext(http.MethodPut, http.StatusServiceUnavailable)
	if err := store.Put(context.Background(), "k", []byte("x"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := second.Object("k"); !ok {
		t.Fatal("second endpoint must hold the object after failover")
	}
}
