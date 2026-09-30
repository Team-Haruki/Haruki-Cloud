package s3

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"haruki-cloud/internal/storage"
)

const (
	// errorBodyLimit bounds how much of an error response is decoded.
	errorBodyLimit = 64 << 10
	// listBodyLimit bounds a ListObjectsV2 page (1000 keys of at most 1024
	// bytes each fit comfortably).
	listBodyLimit = 16 << 20
)

var errInvalidEscape = errors.New("s3: invalid percent escape")

// ResponseError is a non-success S3 answer. Code and Message come from the
// <Error> body when the server sent one (never for HEAD).
type ResponseError struct {
	Op         string
	Key        storage.Key
	Endpoint   string
	StatusCode int
	RetryAfter time.Duration
	Code       string
	Message    string
	bodyErr    error
}

func (e *ResponseError) Unwrap() error { return e.bodyErr }

func (e *ResponseError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "s3: %s %q at %s: HTTP %d", e.Op, e.Key, e.Endpoint, e.StatusCode)
	if e.Code != "" {
		b.WriteString(" " + e.Code)
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	if e.bodyErr != nil {
		b.WriteString(": unreadable error body")
	}
	return b.String()
}

type errorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

// decodeError distinguishes an ordinary empty error body from a truncated
// response. A broken 404 must not certify that an object is absent.
func decodeError(body io.Reader) (code, message string, err error) {
	data, err := io.ReadAll(io.LimitReader(body, errorBodyLimit+1))
	if err != nil {
		return "", "", err
	}
	if len(data) > errorBodyLimit {
		return "", "", errors.New("s3: error response exceeds size limit")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "", "", nil
	}
	var doc errorResponse
	if err := xml.Unmarshal(data, &doc); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(doc.Code), strings.TrimSpace(doc.Message), nil
}

type listBucketResult struct {
	XMLName               xml.Name           `xml:"ListBucketResult"`
	IsTruncated           bool               `xml:"IsTruncated"`
	NextContinuationToken string             `xml:"NextContinuationToken"`
	Contents              []listEntry        `xml:"Contents"`
	CommonPrefixes        []listCommonPrefix `xml:"CommonPrefixes"`
}

type listCommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

type listEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

func decodeList(body io.Reader) (listBucketResult, error) {
	var doc listBucketResult
	if err := xml.NewDecoder(io.LimitReader(body, listBodyLimit)).Decode(&doc); err != nil {
		return listBucketResult{}, fmt.Errorf("s3: decode ListObjectsV2 response: %w", err)
	}
	return doc, nil
}

// parseListTime parses a ListObjectsV2 LastModified value; an unparseable value
// yields the zero time.
func parseListTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func trimETag(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"`)
}
