// Package s3test is an in-memory S3 fake for storage/s3 tests. It verifies
// every signed request by re-signing it with the same signer and rejecting
// mismatches, supports path-style object verbs and ListObjectsV2, and can
// inject failures and latency.
package s3test

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/storage/s3"
)

// VerbList names ListObjectsV2 in FailNext; object verbs use the HTTP method.
const VerbList = "LIST"

const maxBodyBytes = 128 << 20

// Options configures a Server. An empty AccessKey accepts unsigned requests.
type Options struct {
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string // default s3.DefaultRegion
}

// StoredObject is an object held by the Server.
type StoredObject struct {
	Data         []byte
	ContentType  string
	CacheControl string
	ACL          string
	ETag         string
	ModTime      time.Time
}

// Request is a recorded request.
type Request struct {
	Verb   string
	Host   string
	Path   string
	Query  string
	Header http.Header
}

// Server is the S3 fake. Close it when done.
type Server struct {
	*httptest.Server
	opts Options

	mu       sync.Mutex
	objects  map[string]StoredObject
	failures map[string][]int
	hang     time.Duration
	requests []Request
}

// New starts a Server.
func New(opts Options) *Server {
	if opts.Region == "" {
		opts.Region = s3.DefaultRegion
	}
	s := &Server{opts: opts, objects: map[string]StoredObject{}, failures: map[string][]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// FailNext makes the next request of verb (an HTTP method or VerbList) answer
// status with an S3 error body. Calls queue.
func (s *Server) FailNext(verb string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[verb] = append(s.failures[verb], status)
}

// Hang delays every later request by d (or until the client gives up).
func (s *Server) Hang(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hang = d
}

// Object returns the object stored under the full bucket key.
func (s *Server) Object(key string) (StoredObject, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	object, ok := s.objects[key]
	return object, ok
}

// Put seeds an object under the full bucket key.
func (s *Server) Put(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = newObject(data, "application/octet-stream", "", "")
}

// Requests returns the recorded requests in arrival order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func newObject(data []byte, contentType, cacheControl, acl string) StoredObject {
	sum := md5.Sum(data) //nolint:gosec // ETag parity with S3, not security
	return StoredObject{
		Data: slices.Clone(data), ContentType: contentType, CacheControl: cacheControl, ACL: acl,
		ETag: `"` + hex.EncodeToString(sum[:]) + `"`, ModTime: time.Now().UTC().Truncate(time.Second),
	}
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	verb := r.Method
	if key == "" && r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
		verb = VerbList
	}
	status, hang := s.record(verb, r)
	if hang > 0 {
		select {
		case <-time.After(hang):
		case <-r.Context().Done():
			return
		}
	}
	if status != 0 {
		writeError(w, r, status, "InjectedFailure", "injected failure")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "IncompleteBody", err.Error())
		return
	}
	if status, code := s.authorize(r, body); status != 0 {
		writeError(w, r, status, code, "request rejected by s3test")
		return
	}
	if bucket != s.opts.Bucket {
		writeError(w, r, http.StatusNotFound, "NoSuchBucket", "bucket does not exist")
		return
	}
	s.dispatch(w, r, verb, key, body)
}

func (s *Server) record(verb string, r *http.Request) (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{
		Verb: verb, Host: r.Host, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(),
	})
	status := 0
	if queue := s.failures[verb]; len(queue) > 0 {
		status, s.failures[verb] = queue[0], queue[1:]
	}
	return status, s.hang
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request, verb, key string, body []byte) {
	switch {
	case verb == VerbList:
		s.list(w, r)
	case key == "":
		writeError(w, r, http.StatusBadRequest, "InvalidRequest", "object key required")
	case verb == http.MethodPut:
		s.mu.Lock()
		s.objects[key] = newObject(body, r.Header.Get("Content-Type"), r.Header.Get("Cache-Control"), r.Header.Get("X-Amz-Acl"))
		etag := s.objects[key].ETag
		s.mu.Unlock()
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)
	case verb == http.MethodGet || verb == http.MethodHead:
		object, ok := s.Object(key)
		if !ok {
			writeError(w, r, http.StatusNotFound, "NoSuchKey", "key does not exist")
			return
		}
		w.Header().Set("Content-Type", object.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(object.Data)))
		w.Header().Set("ETag", object.ETag)
		w.Header().Set("Last-Modified", object.ModTime.Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
		if verb == http.MethodGet {
			_, _ = w.Write(object.Data)
		}
	case verb == http.MethodDelete:
		s.mu.Lock()
		delete(s.objects, key)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "unsupported verb")
	}
}

// authorize re-signs the request with s3.Sign over the headers the client
// declared signed and compares the Authorization values.
func (s *Server) authorize(r *http.Request, body []byte) (int, string) {
	if s.opts.AccessKey == "" {
		return 0, ""
	}
	auth := r.Header.Get("Authorization")
	credential, signedHeaders, ok := parseAuthorization(auth)
	if !ok {
		return http.StatusForbidden, "AccessDenied"
	}
	if accessKey, _, _ := strings.Cut(credential, "/"); accessKey != s.opts.AccessKey {
		return http.StatusForbidden, "InvalidAccessKeyId"
	}
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash != s3.PayloadSHA256(body) {
		return http.StatusBadRequest, "XAmzContentSHA256Mismatch"
	}
	signedAt, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
	if err != nil {
		return http.StatusForbidden, "AccessDenied"
	}
	replay, err := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), nil)
	if err != nil {
		return http.StatusBadRequest, "InvalidRequest"
	}
	for _, name := range strings.Split(signedHeaders, ";") {
		if name != "host" {
			replay.Header[http.CanonicalHeaderKey(name)] = r.Header.Values(name)
		}
	}
	s3.Sign(replay, payloadHash, s.opts.AccessKey, s.opts.SecretKey, s.opts.Region, "s3", signedAt)
	if replay.Header.Get("Authorization") != auth {
		return http.StatusForbidden, "SignatureDoesNotMatch"
	}
	return 0, ""
}

func parseAuthorization(value string) (credential, signedHeaders string, ok bool) {
	rest, found := strings.CutPrefix(value, "AWS4-HMAC-SHA256 ")
	if !found {
		return "", "", false
	}
	for part := range strings.SplitSeq(rest, ",") {
		name, val, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch name {
		case "Credential":
			credential = val
		case "SignedHeaders":
			signedHeaders = val
		}
	}
	return credential, signedHeaders, credential != "" && signedHeaders != ""
}

type listResult struct {
	XMLName               xml.Name    `xml:"ListBucketResult"`
	Xmlns                 string      `xml:"xmlns,attr"`
	Prefix                string      `xml:"Prefix"`
	KeyCount              int         `xml:"KeyCount"`
	MaxKeys               int         `xml:"MaxKeys"`
	IsTruncated           bool        `xml:"IsTruncated"`
	NextContinuationToken string      `xml:"NextContinuationToken,omitempty"`
	Contents              []listEntry `xml:"Contents"`
}

type listEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int    `xml:"Size"`
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	maxKeys, err := strconv.Atoi(query.Get("max-keys"))
	if err != nil || maxKeys <= 0 || maxKeys > 1000 {
		maxKeys = 1000
	}
	token := query.Get("continuation-token")
	s.mu.Lock()
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) && key >= token {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	result := listResult{Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/", Prefix: prefix, MaxKeys: maxKeys}
	if len(keys) > maxKeys {
		result.IsTruncated, result.NextContinuationToken = true, keys[maxKeys]
		keys = keys[:maxKeys]
	}
	for _, key := range keys {
		object := s.objects[key]
		result.Contents = append(result.Contents, listEntry{
			Key: key, LastModified: object.ModTime.Format("2006-01-02T15:04:05.000Z"), ETag: object.ETag, Size: len(object.Data),
		})
	}
	s.mu.Unlock()
	result.KeyCount = len(result.Contents)
	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, xml.Header)
	_ = xml.NewEncoder(w).Encode(result)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = fmt.Fprintf(w, "%s<Error><Code>%s</Code><Message>%s</Message></Error>", xml.Header, code, message)
}
