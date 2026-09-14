package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	sigV4Algorithm   = "AWS4-HMAC-SHA256"
	amzDateLayout    = "20060102T150405Z"
	amzDayLayout     = "20060102"
	headerAmzDate    = "X-Amz-Date"
	headerAmzContent = "X-Amz-Content-Sha256"
	headerAuth       = "Authorization"
)

// EmptyPayloadSHA256 is the hex SHA-256 of an empty body.
const EmptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// PayloadSHA256 returns the hex SHA-256 of body.
func PayloadSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Sign adds AWS Signature Version 4 authentication to req. It sets Host (when
// empty), X-Amz-Date and X-Amz-Content-Sha256, then Authorization. The signed
// headers are host, every x-amz-* header, and content-type, content-md5 and
// cache-control when present. payloadSHA256 must be the hex SHA-256 of the
// exact body sent; unsigned or chunked payloads are not supported.
func Sign(req *http.Request, payloadSHA256, accessKey, secretKey, region, service string, now time.Time) {
	if req.Host == "" {
		req.Host = req.URL.Host
	}
	now = now.UTC()
	req.Header.Set(headerAmzDate, now.Format(amzDateLayout))
	req.Header.Set(headerAmzContent, payloadSHA256)
	signed := signedHeaderNames(req.Header)
	_, _, signature := signV4(req, signed, payloadSHA256, secretKey, region, service, now)
	scope := credentialScope(now, region, service)
	req.Header.Set(headerAuth, sigV4Algorithm+" Credential="+accessKey+"/"+scope+
		", SignedHeaders="+strings.Join(signed, ";")+", Signature="+signature)
}

// signedHeaderNames returns the sorted lower-case header names Sign covers.
func signedHeaderNames(header http.Header) []string {
	names := []string{"host"}
	for name := range header {
		lower := strings.ToLower(name)
		switch {
		case lower == "host", lower == "authorization":
		case strings.HasPrefix(lower, "x-amz-"),
			lower == "content-type", lower == "content-md5", lower == "cache-control":
			names = append(names, lower)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// signV4 computes the canonical request, string to sign and signature for req
// over the given lower-case, sorted header names. The X-Amz-Date header must
// already be set when it is one of the signed headers.
func signV4(req *http.Request, signed []string, payloadSHA256, secretKey, region, service string, now time.Time) (string, string, string) {
	canonical := canonicalRequest(req, signed, payloadSHA256)
	scope := credentialScope(now, region, service)
	sum := sha256.Sum256([]byte(canonical))
	toSign := sigV4Algorithm + "\n" + now.UTC().Format(amzDateLayout) + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := hmacSHA256([]byte("AWS4"+secretKey), now.UTC().Format(amzDayLayout))
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	return canonical, toSign, hex.EncodeToString(hmacSHA256(key, toSign))
}

func credentialScope(now time.Time, region, service string) string {
	return now.UTC().Format(amzDayLayout) + "/" + region + "/" + service + "/aws4_request"
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func canonicalRequest(req *http.Request, signed []string, payloadSHA256 string) string {
	var b strings.Builder
	b.WriteString(req.Method)
	b.WriteByte('\n')
	b.WriteString(canonicalURI(req.URL.Path))
	b.WriteByte('\n')
	b.WriteString(canonicalQuery(req.URL.RawQuery))
	b.WriteByte('\n')
	for _, name := range signed {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(canonicalHeaderValue(req, name))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(strings.Join(signed, ";"))
	b.WriteByte('\n')
	b.WriteString(payloadSHA256)
	return b.String()
}

func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return encodePath(path)
}

func canonicalHeaderValue(req *http.Request, name string) string {
	if name == "host" {
		if req.Host != "" {
			return req.Host
		}
		return req.URL.Host
	}
	values := req.Header.Values(name)
	trimmed := make([]string, len(values))
	for i, value := range values {
		trimmed[i] = strings.Join(strings.Fields(value), " ")
	}
	return strings.Join(trimmed, ",")
}

// canonicalQuery sorts the raw query's parameters by encoded name, then value,
// re-encoding both per RFC 3986. A parameter that fails to unescape is used
// verbatim.
func canonicalQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	pairs := make([]string, 0, strings.Count(rawQuery, "&")+1)
	for part := range strings.SplitSeq(rawQuery, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		pairs = append(pairs, encodeQueryComponent(unescapeQuery(name))+"="+encodeQueryComponent(unescapeQuery(value)))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, "&")
}

func unescapeQuery(value string) string {
	decoded, err := unescape(value)
	if err != nil {
		return value
	}
	return decoded
}

// unescape decodes %XX sequences; unlike url.QueryUnescape it keeps "+"
// literal, matching how the client encodes spaces as %20.
func unescape(value string) (string, error) {
	if !strings.Contains(value, "%") {
		return value, nil
	}
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			out = append(out, value[i])
			continue
		}
		if i+2 >= len(value) {
			return "", errInvalidEscape
		}
		decoded, err := hex.DecodeString(value[i+1 : i+3])
		if err != nil {
			return "", errInvalidEscape
		}
		out = append(out, decoded[0])
		i += 2
	}
	return string(out), nil
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

func encode(value string, keepSlash bool) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		if isUnreserved(c) || (keepSlash && c == '/') {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return b.String()
}

// encodePath URI-encodes every byte except RFC 3986 unreserved characters and
// "/".
func encodePath(path string) string { return encode(path, true) }

// encodeQueryComponent URI-encodes every byte except RFC 3986 unreserved
// characters.
func encodeQueryComponent(value string) string { return encode(value, false) }
