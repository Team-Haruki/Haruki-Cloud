package s3

import (
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
	Code       string
	Message    string
}

func (e *ResponseError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "s3: %s %q at %s: HTTP %d", e.Op, e.Key, e.Endpoint, e.StatusCode)
	if e.Code != "" {
		b.WriteString(" " + e.Code)
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	return b.String()
}

type errorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

// decodeError reads the S3 <Error> document from body; a missing or malformed
// document yields empty strings.
func decodeError(body io.Reader) (code, message string) {
	var doc errorResponse
	if err := xml.NewDecoder(io.LimitReader(body, errorBodyLimit)).Decode(&doc); err != nil {
		return "", ""
	}
	return strings.TrimSpace(doc.Code), strings.TrimSpace(doc.Message)
}

type listBucketResult struct {
	XMLName               xml.Name    `xml:"ListBucketResult"`
	IsTruncated           bool        `xml:"IsTruncated"`
	NextContinuationToken string      `xml:"NextContinuationToken"`
	Contents              []listEntry `xml:"Contents"`
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
