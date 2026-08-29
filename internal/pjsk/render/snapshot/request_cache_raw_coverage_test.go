//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package snapshot

import (
	"context"
	"errors"
	"testing"
)

func TestRawValueErrorAndCopyBranches(t *testing.T) {
	var nilService *Service
	if _, err := nilService.RawValue("key"); err == nil {
		t.Fatal("nil service raw value unexpectedly succeeded")
	}
	for _, tc := range []struct {
		name    string
		service *Service
		key     string
	}{
		{"empty raw", &Service{}, "key"},
		{"empty key", &Service{rawJSON: []byte(`{"key":1}`)}, " "},
		{"invalid json", &Service{rawJSON: []byte(`{`)}, "key"},
		{"missing key", &Service{rawJSON: []byte(`{"other":1}`)}, "key"},
		{"empty value", &Service{rawJSON: []byte(`{"key":null}`)}, "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.service.RawValue(tc.key); err == nil {
				t.Fatal("RawValue unexpectedly succeeded")
			}
		})
	}
	service := &Service{rawJSON: []byte(`{"key":{"value":1}}`)}
	first, err := service.RawValue(" key ")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'x'
	second, err := service.RawValue("key")
	if err != nil || string(second) != `{"value":1}` {
		t.Fatalf("RawValue did not return an owned copy: %q, %v", second, err)
	}
}

func TestRequestCacheContextAndFetchBranches(t *testing.T) {
	if cacheFromContext(nil) != nil {
		t.Fatal("nil context returned a cache")
	}
	wrong := context.WithValue(context.Background(), requestCacheContextKey{}, "wrong")
	if cacheFromContext(wrong) != nil {
		t.Fatal("wrong context value returned a cache")
	}
	ctx := WithRequestCache(nil)
	if cacheFromContext(ctx) == nil {
		t.Fatal("nil context was not initialized with a cache")
	}
	if got := WithRequestCache(ctx); got != ctx {
		t.Fatal("existing request cache context was wrapped again")
	}

	key := privateDataCacheKey{Server: "jp", DataType: "suite", UserID: 1}
	calls := 0
	data, err, hit := cachedPrivateData(context.Background(), key, func() ([]byte, error) {
		calls++
		return []byte("direct"), nil
	})
	if err != nil || hit || string(data) != "direct" || calls != 1 {
		t.Fatalf("direct fetch = %q,%v hit=%v calls=%d", data, err, hit, calls)
	}

	data, err, hit = cachedPrivateData(ctx, key, func() ([]byte, error) {
		calls++
		return []byte("cached"), nil
	})
	if err != nil || hit || string(data) != "cached" {
		t.Fatalf("first cached fetch = %q,%v hit=%v", data, err, hit)
	}
	data[0] = 'X'
	data, err, hit = cachedPrivateData(ctx, key, func() ([]byte, error) {
		t.Fatal("cached fetch function called twice")
		return nil, nil
	})
	if err != nil || !hit || string(data) != "cached" {
		t.Fatalf("second cached fetch = %q,%v hit=%v", data, err, hit)
	}

	wantErr := errors.New("fetch failed")
	errorKey := privateDataCacheKey{Server: "en", DataType: "mysekai", UserID: 2}
	data, err, hit = cachedPrivateData(ctx, errorKey, func() ([]byte, error) { return nil, wantErr })
	if data != nil || !errors.Is(err, wantErr) || hit {
		t.Fatalf("cached error fetch = %q,%v hit=%v", data, err, hit)
	}

	manualCache := &requestCache{privateData: map[privateDataCacheKey]*privateDataCacheEntry{key: nil}}
	manualCtx := context.WithValue(context.Background(), requestCacheContextKey{}, manualCache)
	if data, err, hit := cachedPrivateData(manualCtx, key, func() ([]byte, error) { return []byte("filled"), nil }); err != nil || !hit || string(data) != "filled" {
		t.Fatalf("nil cache entry refill = %q,%v hit=%v", data, err, hit)
	}
}
