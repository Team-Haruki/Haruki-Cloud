package s3test_test

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/s3"
	"haruki-cloud/internal/storage/s3/s3test"
)

func newServer(t *testing.T, accessKey string) *s3test.Server {
	t.Helper()
	server := s3test.New(s3test.Options{Bucket: "b", AccessKey: accessKey, SecretKey: "sk"})
	t.Cleanup(server.Close)
	return server
}

func do(t *testing.T, server *s3test.Server, method, target string, body string, sign func(*http.Request, string)) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if sign != nil {
		sign(req, body)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, string(data)
}

func signWith(accessKey, secretKey string) func(*http.Request, string) {
	return func(req *http.Request, body string) {
		s3.Sign(req, s3.PayloadSHA256([]byte(body)), accessKey, secretKey, s3.DefaultRegion, "s3", time.Now())
	}
}

func errorCode(t *testing.T, body string) string {
	t.Helper()
	var doc struct {
		Code string `xml:"Code"`
	}
	if err := xml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("error body %q: %v", body, err)
	}
	return doc.Code
}

func TestServerAuthorization(t *testing.T) {
	server := newServer(t, "ak")
	cases := []struct {
		name   string
		sign   func(*http.Request, string)
		status int
		code   string
	}{
		{"unsigned", nil, http.StatusForbidden, "AccessDenied"},
		{"wrong access key", signWith("other", "sk"), http.StatusForbidden, "InvalidAccessKeyId"},
		{"wrong secret", signWith("ak", "bad"), http.StatusForbidden, "SignatureDoesNotMatch"},
		{"payload mismatch", func(req *http.Request, _ string) {
			s3.Sign(req, s3.EmptyPayloadSHA256, "ak", "sk", s3.DefaultRegion, "s3", time.Now())
		}, http.StatusBadRequest, "XAmzContentSHA256Mismatch"},
		{"bad date", func(req *http.Request, body string) {
			signWith("ak", "sk")(req, body)
			req.Header.Set("X-Amz-Date", "yesterday")
		}, http.StatusForbidden, "AccessDenied"},
		{"no signed headers", func(req *http.Request, _ string) {
			req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=ak/x")
		}, http.StatusForbidden, "AccessDenied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := do(t, server, http.MethodPut, "/b/k", "data", tc.sign)
			if resp.StatusCode != tc.status || errorCode(t, body) != tc.code {
				t.Fatalf("status=%d body=%s", resp.StatusCode, body)
			}
		})
	}
	resp, _ := do(t, server, http.MethodPut, "/b/k", "data", signWith("ak", "sk"))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") == "" {
		t.Fatalf("signed PUT status=%d", resp.StatusCode)
	}
}

func TestServerRoutingAndHelpers(t *testing.T) {
	server := newServer(t, "")
	server.Put("seed/a", []byte("A"))
	if object, ok := server.Object("seed/a"); !ok || string(object.Data) != "A" || object.ETag == "" {
		t.Fatalf("seeded object = %+v", object)
	}
	resp, body := do(t, server, http.MethodGet, "/other/k", "", nil)
	if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "NoSuchBucket" {
		t.Fatalf("wrong bucket status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = do(t, server, http.MethodPut, "/b/", "x", nil)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "InvalidRequest" {
		t.Fatalf("bucket PUT status=%d body=%s", resp.StatusCode, body)
	}
	resp, body = do(t, server, http.MethodPost, "/b/k", "x", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed || errorCode(t, body) != "MethodNotAllowed" {
		t.Fatalf("POST status=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = do(t, server, http.MethodHead, "/b/missing", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("HEAD missing status=%d", resp.StatusCode)
	}
	server.FailNext(s3test.VerbList, http.StatusInternalServerError)
	resp, body = do(t, server, http.MethodGet, "/b?list-type=2", "", nil)
	if resp.StatusCode != http.StatusInternalServerError || errorCode(t, body) != "InjectedFailure" {
		t.Fatalf("injected status=%d body=%s", resp.StatusCode, body)
	}
	requests := server.Requests()
	if len(requests) != 5 || requests[4].Verb != s3test.VerbList || requests[0].Path != "/other/k" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestServerListPagination(t *testing.T) {
	server := newServer(t, "")
	for _, key := range []string{"p/1", "p/2", "p/3", "q/1"} {
		server.Put(key, []byte(key))
	}
	var tokens []string
	var keys []string
	token := ""
	for page := 0; page < 5; page++ {
		target := "/b?list-type=2&max-keys=2&prefix=p%2F"
		if token != "" {
			target += "&continuation-token=" + token
		}
		resp, body := do(t, server, http.MethodGet, target, "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list status=%d", resp.StatusCode)
		}
		var doc struct {
			IsTruncated           bool
			NextContinuationToken string
			Contents              []struct{ Key string }
		}
		if err := xml.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		for _, c := range doc.Contents {
			keys = append(keys, c.Key)
		}
		if !doc.IsTruncated {
			break
		}
		token = doc.NextContinuationToken
		tokens = append(tokens, token)
	}
	if strings.Join(keys, ",") != "p/1,p/2,p/3" || strings.Join(tokens, ",") != "p/3" {
		t.Fatalf("keys=%v tokens=%v", keys, tokens)
	}
	resp, body := do(t, server, http.MethodGet, "/b?list-type=2&max-keys=bogus", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "<MaxKeys>1000</MaxKeys>") {
		t.Fatalf("default max-keys status=%d body=%s", resp.StatusCode, body)
	}
}

func TestServerHang(t *testing.T) {
	server := newServer(t, "")
	server.Hang(time.Second)
	store, err := s3.New(s3.Config{Endpoints: []string{server.URL}, Bucket: "b", PathStyle: true, StatTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := store.Stat(context.Background(), "k"); err == nil {
		t.Fatal("Stat against a hanging server must time out")
	}
	if time.Since(start) > 900*time.Millisecond {
		t.Fatal("hang was not bounded by the client timeout")
	}
	server.Hang(0)
	if _, err := store.Stat(context.Background(), "k"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("Stat after hang error = %v", err)
	}
}

func TestServerObjectRoundTrip(t *testing.T) {
	server := newServer(t, "ak")
	store, err := s3.New(s3.Config{
		Endpoints: []string{server.URL}, Bucket: "b", AccessKey: "ak", SecretKey: "sk", PathStyle: true, DefaultACL: "public-read",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, "dir/x.txt", []byte("hello"), storage.PutOptions{ContentType: "text/plain", CacheControl: "no-store"}); err != nil {
		t.Fatal(err)
	}
	object, ok := server.Object("dir/x.txt")
	if !ok || object.ContentType != "text/plain" || object.CacheControl != "no-store" || object.ACL != "public-read" {
		t.Fatalf("stored = %+v", object)
	}
	if data, err := store.Get(ctx, "dir/x.txt"); err != nil || string(data) != "hello" {
		t.Fatalf("Get = %q, %v", data, err)
	}
	stat, err := store.Stat(ctx, "dir/x.txt")
	if err != nil || stat.Size != 5 || stat.ETag == "" || stat.ModTime.IsZero() {
		t.Fatalf("Stat = %+v, %v", stat, err)
	}
	if err := store.Delete(ctx, "dir/x.txt"); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.Object("dir/x.txt"); ok {
		t.Fatal("object survived Delete")
	}
	if _, err := store.Get(ctx, "dir/x.txt"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("Get after Delete error = %v", err)
	}
}
